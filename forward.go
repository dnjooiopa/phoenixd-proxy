package main

import (
	"bytes"
	"crypto/tls"
	"log"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
}

func ForwardToAll(endpoints []Endpoint, body []byte, headers map[string]string) {
	var g errgroup.Group
	for _, ep := range endpoints {
		g.Go(func() error {
			req, err := http.NewRequest(http.MethodPost, ep.URL, bytes.NewReader(body))
			if err != nil {
				log.Printf("forward to %s: failed to create request: %v\n", ep.URL, err)
				return nil
			}

			for k, v := range headers {
				req.Header.Set(k, v)
			}

			resp, err := httpClient.Do(req)
			if err != nil {
				log.Printf("forward to %s failed: %v", ep.URL, err)
				return nil
			}
			defer resp.Body.Close()

			log.Printf("forwarded to %s: %d", ep.URL, resp.StatusCode)
			return nil
		})
	}

	g.Wait()
}
