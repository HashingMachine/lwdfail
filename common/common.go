package common

import (
	"gorm.io/gorm"
	"time"
)

type Server struct {
	gorm.Model  `json:"-"`
	Address     string    `json:"address"`
	Blockchain  string    `json:"blockchain"`
	Height      uint64    `json:"height"`
	LastChecked time.Time `json:"-"`
	Up          bool      `json:"up"`
	Validated   bool      `json:"-"`
}
