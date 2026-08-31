package github

import (
	"errors"
	"os"

	"github.com/google/go-github/v60/github"
)

type Client struct {
	GH *github.Client
}

func NewClient() (*Client, error) {
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		return nil, errors.New("GH_TOKEN environment variable is not set")
	}

	client := github.NewClient(nil).WithAuthToken(token)

	return &Client{
		GH: client,
	}, nil
}
