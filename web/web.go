// Copyright (c) 2017 Townsourced Inc.

package web

import (
	"compress/gzip"
	"crypto/tls"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	ReadTimeout       string
	WriteTimeout      string
	MaxHeaderBytes    int
	MinTLSVersion     uint16
	CertFile          string
	KeyFile           string
	MaxUploadMemoryMB int
	Port              int
}

// DefaultConfig returns the default configuration for the web layer
func DefaultConfig() Config {
	return Config{
		MinTLSVersion:     tls.VersionTLS10,
		MaxUploadMemoryMB: 10, //10MB default
	}
}

const (
	strictTransportSecurity = "max-age=86400"
)

var (
	zipPool         sync.Pool
	maxUploadMemory = int64(10 << 20)
	isSSL           = false
)

func init() {
	zipPool = sync.Pool{
		New: func() interface{} {
			return gzip.NewWriter(nil)
		},
	}
}

func StartServer(cfg Config) error {
	if cfg.MaxUploadMemoryMB <= 0 {
		cfg.MaxUploadMemoryMB = DefaultConfig().MaxUploadMemoryMB
	}
	maxUploadMemory = int64(cfg.MaxUploadMemoryMB) << 20

	var readTimeout time.Duration
	var writeTimeout time.Duration
	var err error

	if cfg.ReadTimeout != "" {
		readTimeout, err = time.ParseDuration(cfg.ReadTimeout)
		if err != nil {
			log.Printf("Invalid ReadTimeout duration format (%s), using default", cfg.ReadTimeout)
		}
	}
	if cfg.WriteTimeout != "" {
		writeTimeout, err = time.ParseDuration(cfg.WriteTimeout)
		if err != nil {
			log.Printf("Invalid WriteTimeout duration format (%s), using default", cfg.WriteTimeout)
		}
	}

	tlsCFG := &tls.Config{MinVersion: cfg.MinTLSVersion}

	server := &http.Server{
		Handler:        setupRoutes(),
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		MaxHeaderBytes: cfg.MaxHeaderBytes,
		ErrorLog:       nil,
	}

	//TODO: Error log handling

	if cfg.CertFile == "" || cfg.KeyFile == "" {
		if cfg.Port <= 0 {
			cfg.Port = 80
		}
		log.Printf("Lex Library is running on port %d", cfg.Port)
		server.Addr = ":" + strconv.Itoa(cfg.Port)
		err = server.ListenAndServe()
	} else {
		if cfg.Port <= 0 {
			cfg.Port = 443
		}

		isSSL = true
		server.TLSConfig = tlsCFG

		log.Printf("Lex Library is running on port %d", cfg.Port)
		server.Addr = ":" + strconv.Itoa(cfg.Port)
		err = server.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
	}

	return err
}

func standardHeaders(w http.ResponseWriter) {
	if isSSL {
		w.Header().Set("Strict-Transport-Security", strictTransportSecurity)
	}
}

// gzipReponse gzips the response data for any respones writers defined to use it
type gzipResponse struct {
	zip *gzip.Writer
	http.ResponseWriter
}

func (g *gzipResponse) Write(b []byte) (int, error) {
	if g.zip == nil {
		return g.ResponseWriter.Write(b)
	}
	return g.zip.Write(b)
}

func (g *gzipResponse) Close() error {
	if g.zip == nil {
		return nil
	}
	err := g.zip.Close()
	if err != nil {
		return err
	}
	zipPool.Put(g.zip)
	return nil
}

func responseWriter(w http.ResponseWriter, r *http.Request) *gzipResponse {
	var writer *gzip.Writer

	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gz := zipPool.Get().(*gzip.Writer)
		gz.Reset(w)

		writer = gz
	}

	return &gzipResponse{zip: writer, ResponseWriter: w}
}
