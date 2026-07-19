package usecase

import (
	"context"

	"downstream_client_not_response/client"
)

type UseCase interface {
	Get(ctx context.Context) (string, error)
}

type impl struct{ c *client.Client }

func New() UseCase { return &impl{c: &client.Client{}} }

func (u *impl) Get(ctx context.Context) (string, error) {
	resp, err := u.c.Fetch(&client.FetchRequest{Amount: 1})
	if err != nil {
		return "", err
	}
	return resp.Key, nil
}
