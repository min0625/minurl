// Copyright 2024 The MinURL Authors
package main

import (
	"context"
	"log"

	"github.com/microsoft/kiota-abstractions-go/authentication"
	kiotahttp "github.com/microsoft/kiota-http-go"
	"github.com/min0625/minurl/pkg/kiota/go/gen/client"
	"github.com/min0625/minurl/pkg/kiota/go/gen/client/models"
)

func main() {
	authProvider := &authentication.AnonymousAuthenticationProvider{}

	adapter, err := kiotahttp.NewNetHttpRequestAdapter(authProvider)
	if err != nil {
		log.Fatalf("create request adapter: %v", err)
	}

	adapter.SetBaseUrl("http://localhost:8888")

	c := client.NewMinURLClient(adapter)
	ctx := context.Background()

	// create a new short URL
	body := models.NewShortURL()
	originalURL := "https://example.com/very/long/path?query=value"
	body.SetOriginalUrl(&originalURL)

	created, err := c.Api().V1().Urls().Post(ctx, body, nil)
	if err != nil {
		log.Fatalf("create short URL failed: %v", err)
	}

	log.Printf(
		"short URL created: id=%s, original_url=%s\n",
		*created.GetId(),
		*created.GetOriginalUrl(),
	)

	// fetch the newly created short URL
	fetched, err := c.Api().V1().Urls().ById(*created.GetId()).Get(ctx, nil)
	if err != nil {
		log.Fatalf("fetch short URL failed: %v", err)
	}

	log.Printf("fetch result: id=%s, original_url=%s, create_time=%v\n",
		*fetched.GetId(), *fetched.GetOriginalUrl(), fetched.GetCreateTime())
}
