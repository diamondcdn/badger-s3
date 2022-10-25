package main

import (
	"fmt"
	badgers3 "github.com/diamondcdn/badger-s3"
	"log"
	"net/http"

	"github.com/caddyserver/certmagic"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "Hello, HTTPS visitor!")
	})

	var err error
	certmagic.DefaultACME.Email = "yourname@example.com"
	certmagic.DefaultACME.CA = certmagic.LetsEncryptStagingCA
	certmagic.Default.Storage, err = badgers3.NewS3Storage(badgers3.S3Opts{
		Endpoint:        "very-cool.s3.backblazeb2.com",
		Bucket:          "your-crypto-bucket",
		AccessKeyID:     "some-key",
		SecretAccessKey: "some-secret",
		ObjPrefix:       "all-objects-will-start-with-this",
		EncryptionKey:   []byte("supersecretkeyofexactly32bytes!!"),
	})
	if err != nil {
		log.Fatal(err)
	}

	err = certmagic.HTTPS([]string{"your.domain.example.com"}, nil)
	if err != nil {
		log.Fatal(err)
	}
}
