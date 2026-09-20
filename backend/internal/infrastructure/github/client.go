package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
)

type Client struct {
	httpClient *http.Client
	tokenURL   string
	apiURL     string
}

func New(httpClient *http.Client, tokenURL, apiURL string) *Client {
	return &Client{httpClient: httpClient, tokenURL: tokenURL, apiURL: strings.TrimRight(apiURL, "/")}
}

func (client *Client) Authenticate(ctx context.Context, clientID, clientSecret, code, verifier string) (appauth.Profile, error) {
	form := url.Values{
		"client_id": {clientID}, "client_secret": {clientSecret}, "code": {code}, "code_verifier": {verifier},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.tokenURL, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return appauth.Profile{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return appauth.Profile{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return appauth.Profile{}, fmt.Errorf("token endpoint status %d", response.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil {
		return appauth.Profile{}, err
	}
	if token.AccessToken == "" || token.Error != "" {
		return appauth.Profile{}, errors.New("GitHub did not issue an access token")
	}

	request, err = http.NewRequestWithContext(ctx, http.MethodGet, client.apiURL+"/user", nil)
	if err != nil {
		return appauth.Profile{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err = client.httpClient.Do(request)
	if err != nil {
		return appauth.Profile{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return appauth.Profile{}, fmt.Errorf("profile endpoint status %d", response.StatusCode)
	}
	var profile struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&profile); err != nil {
		return appauth.Profile{}, err
	}
	return appauth.Profile{ID: profile.ID, Login: profile.Login, DisplayName: profile.Name, AvatarURL: profile.AvatarURL}, nil
}
