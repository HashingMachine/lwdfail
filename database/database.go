package database

import (
	"lwdfail/common"
	"regexp"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var db *gorm.DB

var addrRe = regexp.MustCompile(`https?:\/\/((\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})|([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+([a-zA-Z]{2,6}))(:\d{1,5})$`)

func Init(filename string) (err error) {
	db, err = gorm.Open(sqlite.Open(filename), &gorm.Config{})
	if err != nil {
		return err
	}

	err = db.AutoMigrate(&common.Server{})
	return err
}

func AddServer(addr string) {
	var server common.Server
	server.Address = addr
	db.Create(&server)
}

func RemoveServer(server common.Server) {
	db.Delete(&server)
}

func UpdateServer(server common.Server) {
	db.Save(server)
}

func ListServers(unvalidated, down bool) []common.Server {
	var servers []common.Server
	if !unvalidated || !down {
		var s common.Server
		var fields []string
		if !unvalidated {
			s.Validated = true
			fields = append(fields, "validated")
		}
		if !down {
			s.Up = true
			fields = append(fields, "up")
		}
		db.Where(s, fields).Find(&servers)
	} else {
		db.Find(&servers)
	}

	return servers
}

func IsKnownAddr(addr string) bool {
	addr = addrRe.FindStringSubmatch(addr)[1]

	var knownServers []common.Server
	db.Find(&knownServers)

	for _, server := range knownServers {
		knownAddr := addrRe.FindStringSubmatch(server.Address)[1]
		if knownAddr == addr {
			return true
		}
	}
	return false
}
