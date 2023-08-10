package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"lwdfail/common"
	"lwdfail/database"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/zcash/lightwalletd/walletrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const dbFilename = "db.sqlite"

//go:embed templates/*.html templates/*.tmpl
var fs embed.FS

var (
	httpRe   = regexp.MustCompile(`https?:\/\/`)
	ipAddrRe = regexp.MustCompile(`^https?:\/\/\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d{1,5})$`)
	domainRe = regexp.MustCompile(`^https?:\/\/([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+([a-zA-Z]{2,6})(:\d{1,5})$`)
)

func isValidAddr(addr string) bool {
	return ipAddrRe.MatchString(addr) || domainRe.MatchString(addr)
}

func timeSince(t time.Time) string {
	d := time.Since(t)

	hours := int(d.Hours())
	days := hours / 24
	minutes := int(d.Minutes())
	seconds := int(d.Seconds())

	var s string
	if days != 0 {
		s = fmt.Sprintf("%d days ago", days)
	} else if hours != 0 {
		s = fmt.Sprintf("%d hours ago", hours)
	} else if minutes != 0 {
		s = fmt.Sprintf("%d minutes ago", minutes)
	} else if seconds != 0 {
		s = fmt.Sprintf("%d seconds ago", seconds)
	} else {
		s = "now"
	}

	return s
}

func index(c *gin.Context) {
	servers := database.ListServers(false, true)
	c.HTML(http.StatusOK, "index.html.tmpl", gin.H{"servers": servers})
}

func serverList(c *gin.Context) {
	servers := database.ListServers(false, true)
	c.JSON(http.StatusOK, gin.H{"servers": servers})
}

func addServer(c *gin.Context) {
	addr := c.PostForm("address")
	if addr == "" {
		c.HTML(http.StatusOK, "simple_message.html.tmpl", gin.H{"msg": "The address must not be empty."})
		return
	}

	if !isValidAddr(addr) {
		c.HTML(http.StatusOK, "simple_message.html.tmpl", gin.H{"msg": "Invalid address."})
		return
	}

	if database.IsKnownAddr(addr) {
		c.HTML(http.StatusOK, "simple_message.html.tmpl", gin.H{"msg": "Server already known."})
		return
	}

	database.AddServer(addr)

	c.HTML(http.StatusOK, "simple_message.html.tmpl", gin.H{"msg": "Server successfully added!"})
}

func checkServer(server *common.Server) error {
	defer func() { server.LastChecked = time.Now() }()

	var cred credentials.TransportCredentials
	if strings.HasPrefix(server.Address, "http://") {
		cred = insecure.NewCredentials()
	} else {
		cred = credentials.NewTLS(nil)
	}

	addr := httpRe.ReplaceAllString(server.Address, "")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(cred))
	if err != nil {
		return err
	}
	defer conn.Close()

	client := walletrpc.NewCompactTxStreamerClient(conn)

	info, err := client.GetLightdInfo(ctx, &walletrpc.Empty{})
	if err != nil {
		return err
	}

	server.Blockchain = info.ChainName
	server.Height = info.BlockHeight
	server.Up = true

	return nil
}

func checkServers(interval int) {
	t := time.NewTicker(time.Minute * time.Duration(interval))
	for {
		<-t.C

		servers := database.ListServers(true, true)
		for _, server := range servers {
			log.WithFields(log.Fields{"server": server.Address}).Debug("checking server")

			if err := checkServer(&server); err != nil {
				if !server.Validated {
					log.WithFields(log.Fields{
						"server": server.Address,
						"reason": err.Error(),
					}).Debug("removing server")
					database.RemoveServer(server)
					continue
				} else {
					log.WithFields(log.Fields{
						"server": server.Address,
						"reason": err.Error(),
					}).Debug("marking server as offline")
				}
				server.Up = false
			}
			server.Validated = true

			database.UpdateServer(server)
		}
	}
}

func handle404(c *gin.Context) {
	if c.Writer.Status() == 404 {
		c.Redirect(http.StatusMovedPermanently, "/")
	}
	c.Next()
}

func main() {
	debug := false
	debugEnv := os.Getenv("DEBUG")
	if debugEnv == "1" || debugEnv == "true" {
		debug = true
	}

	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(handle404)

	if debug {
		log.SetLevel(log.DebugLevel)
	}

	if err := database.Init(dbFilename); err != nil {
		log.Fatal(err)
	}

	checkInterval := os.Getenv("CHECK_INTERVAL")
	if checkInterval == "" {
		checkInterval = "30"
	}

	interval, err := strconv.Atoi(checkInterval)
	if err != nil {
		log.Fatal("invalid CHECK_INTERVAL specified")
	}

	go checkServers(interval)

	funcs := template.FuncMap{"timeSince": timeSince}
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFS(fs, "templates/*"))
	r.SetHTMLTemplate(tmpl)

	r.GET("/", index)
	r.GET("/servers.json", serverList)
	r.GET("/faq", func(c *gin.Context) {
		c.HTML(http.StatusOK, "faq.html", nil)
	})
	r.GET("/contact", func(c *gin.Context) {
		c.HTML(http.StatusOK, "contact.html", nil)
	})
	r.POST("/add", addServer)
	log.Fatal(r.Run())
}
