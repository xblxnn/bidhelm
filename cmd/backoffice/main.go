package main

import (
	"bidhelm/internal/app"
	"bidhelm/internal/backoffice"
	"bidhelm/internal/store"
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	s, e := store.Open(app.Env("DB_PATH", "data/app.db"))
	if e != nil {
		log.Fatal(e)
	}
	defer s.DB.Close()
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "has-admin":
			if !s.HasAdmin() {
				os.Exit(1)
			}
			return
		case "init":
			if s.HasAdmin() {
				fmt.Println("Administrator already exists; unchanged")
				return
			}
			if len(os.Args) != 3 {
				log.Fatal("usage: backoffice init LOGIN; password is read from stdin")
			}
			pw, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if e = s.Init(os.Args[2], strings.TrimRight(pw, "\r\n")); e != nil {
				log.Fatal(e)
			}
			fmt.Println("Administrator created")
			return
		case "reset-password":
			pw, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if e = s.Password(strings.TrimRight(pw, "\r\n")); e != nil {
				log.Fatal(e)
			}
			fmt.Println("Password changed; sessions revoked")
			return
		case "backup":
			if len(os.Args) != 3 {
				log.Fatal("usage: backoffice backup DESTINATION")
			}
			if e = s.Backup(os.Args[2]); e != nil {
				log.Fatal(e)
			}
			fmt.Println("Backup created")
			return
		default:
			log.Fatal("unknown command")
		}
	}
	if !s.HasAdmin() {
		log.Fatal("initialize administrator first")
	}
	ctx, cancel := app.Context()
	defer cancel()
	p := &backoffice.Publisher{Store: s, URL: app.Env("DSP_INTERNAL_URL", "http://127.0.0.1:8082"), Token: app.Token(), Client: &http.Client{Timeout: 3 * time.Second}}
	go p.Run(ctx)
	web := &backoffice.Server{Store: s, Publisher: p, BaseURL: app.Env("PUBLIC_URL", "http://127.0.0.1:8080"), Secure: strings.HasPrefix(app.Env("PUBLIC_URL", ""), "https://")}
	log.Print("backoffice listening on localhost:8081")
	if e = app.Serve(ctx, app.Server("127.0.0.1:8081", web.Handler())); e != nil {
		log.Fatal(e)
	}
}
