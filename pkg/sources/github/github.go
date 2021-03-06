/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/knative/eventing/pkg/sources"
	"golang.org/x/oauth2"

	ghclient "github.com/google/go-github/github"
)

const (
	webhookIDKey = "id"
	ownerKey     = "owner"
	repoKey      = "repo"

	// SuccessSynced is used as part of the Event 'reason' when a Feed is synced
	SuccessSynced = "Synced"

	// MessageResourceSynced is the message used for an Event fired when a Feed
	// is synced successfully
	MessageResourceSynced = "Feed synced successfully"
)

type GithubEventSource struct {
}

func NewGithubEventSource() sources.EventSource {
	return &GithubEventSource{}
}

func (t *GithubEventSource) StopFeed(trigger sources.EventTrigger, feedContext sources.FeedContext) error {
	log.Printf("Stopping github webhook feed with context %+v", feedContext)

	components := strings.Split(trigger.Resource, "/")
	owner := components[0]
	repo := components[1]

	if _, ok := feedContext.Context[webhookIDKey]; !ok {
		// there's no webhook id, nothing to do.
		log.Printf("No Webhook ID Found, bailing...")
		return nil
	}
	webhookID := feedContext.Context[webhookIDKey].(string)

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: trigger.Parameters["accessToken"].(string)},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := ghclient.NewClient(tc)

	id, err := strconv.ParseInt(webhookID, 10, 64)
	if err != nil {
		log.Printf("Failed to convert webhook %q to int64 : %s", webhookID, err)
		return err
	}
	_, err = client.Repositories.DeleteHook(ctx, owner, repo, id)
	if err != nil {
		if errResp, ok := err.(*ghclient.ErrorResponse); ok {
			// If the webhook doesn't exist, nothing to do
			if errResp.Message == "Not Found" {
				log.Printf("Webhook doesn't exist, nothing to delete.")
				return nil
			}
		}
		log.Printf("Failed to delete the webhook: %#v", err)
		return err
	}
	log.Printf("Deleted webhook %q successfully", webhookID)
	return nil
}

func (t *GithubEventSource) StartFeed(trigger sources.EventTrigger, route string) (*sources.FeedContext, error) {
	log.Printf("CREATING GITHUB WEBHOOK")

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: trigger.Parameters["accessToken"].(string)},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := ghclient.NewClient(tc)
	active := true
	name := "web"
	config := make(map[string]interface{})
	config["url"] = fmt.Sprintf("http://%s", route)
	config["content_type"] = "json"
	config["secret"] = trigger.Parameters["secretToken"].(string)
	hook := ghclient.Hook{
		Name:   &name,
		URL:    &route,
		Events: []string{"pull_request"},
		Active: &active,
		Config: config,
	}

	components := strings.Split(trigger.Resource, "/")
	owner := components[0]
	repo := components[1]
	h, _, err := client.Repositories.CreateHook(ctx, owner, repo, &hook)
	if err != nil {
		log.Printf("Failed to create the webhook: %s", err)
		return nil, err
	}
	log.Printf("Created hook: %+v", h)
	return &sources.FeedContext{
		Context: map[string]interface{}{
			webhookIDKey: strconv.FormatInt(*h.ID, 10),
		}}, nil
}

func main() {
	flag.Parse()

	sources.RunEventSource(NewGithubEventSource())
	log.Printf("Done...")
}
