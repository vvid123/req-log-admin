package main

import (
	"github.com/zxyao/req-log-mid/admin"
)

func main() {
	admin.Start(admin.StartOptions{
		ConfigPath: "config.yaml",
		Port:       8080,
	})
}
