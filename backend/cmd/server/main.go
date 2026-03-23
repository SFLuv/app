package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/SFLuv/app/backend/bootstrap"
)

func main() {
	bootstrap.LoadEnv()
	ctx := context.Background()

	pools, err := bootstrap.OpenDBPools(true)
	if err != nil {
		log.Fatal(err)
	}
	defer pools.Close()

	appLogger, err := bootstrap.NewAppLogger()
	if err != nil {
		log.Fatal(fmt.Sprintf("error initializing app logger: %s", err))
	}
	defer appLogger.Close()

	handler, err := bootstrap.NewServerHandler(ctx, pools, appLogger)
	if err != nil {
		log.Fatal(err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")
	tlsPort := os.Getenv("TLS_PORT")
	if tlsPort == "" {
		tlsPort = "8443"
	}
	if certFile != "" && keyFile != "" {
		go func() {
			fmt.Printf("now listening on TLS port %s\n", tlsPort)
			if err := http.ListenAndServeTLS(fmt.Sprintf(":%s", tlsPort), certFile, keyFile, handler); err != nil {
				fmt.Println(err)
			}
		}()
	}

	fmt.Printf("now listening on port %s\n", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), handler); err != nil {
		fmt.Println(err)
	}
}
