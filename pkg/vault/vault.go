package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	http "github.com/hashicorp/go-retryablehttp"
	"io/ioutil"
)

type Client struct {
	token   string
	address string
	http    *http.Client
}

func readJwt(path string) (string, error) {
	f, err := ioutil.ReadFile(path)
	if err != nil {
		return "", errors.New(fmt.Sprintf("failed to read file: %s", err))
	}
	return string(bytes.TrimSpace(f)), nil
}

func NewClient(path, address, role string) (*Client, error) {
	h := http.NewClient()
	c := Client{
		address: address,
		http:    h,
	}

	j, err := readJwt(path)
	if err != nil {
		return &c, fmt.Errorf("could not read jwt token: %s", err)
	}

	body := []byte(fmt.Sprintf(`{"role": "%s", "jwt": "%s"}`, role, j))
	resp, err := h.Post(c.address+"/v1/auth/kubernetes/login", "application/json", body)
	if err != nil {
		return &c, errors.New(fmt.Sprintf("fucked %s", err))
	}

	temp := struct {
		Auth struct {
			Token string `json:"client_token"`
		}
	}{}

	if err := json.NewDecoder(resp.Body).Decode(&temp); err != nil {
		return &c, fmt.Errorf("error decoding response body: %s", err)
	}

	c.token = temp.Auth.Token
	if c.token == "" {
		return &c, errors.New("vault token is missing")
	}
	return &c, nil
}

func (c *Client) GetSecret(path, key string) (string, error) {

	req, err := http.NewRequest("POST", c.address+"/v1/secret/data/"+path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to make a request to vault: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vault-Token", c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request to vault: %s", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read the response body: %s", err)
	}

	temp := struct {
		Data struct {
			Data map[string]string `json:"data"`
		}
	}{}

	if err := json.Unmarshal(body, &temp); err != nil {
		return "", fmt.Errorf("failed unmarshal response body: %s", err)
	}

	if len(temp.Data.Data) < 1 {
		return "", fmt.Errorf("no secrets were found with vault path: %s", path)
	}

	if _, ok := temp.Data.Data[key]; !ok {
		return "", errors.New(fmt.Sprintf("no key found %s in vault path %s", key, path))
	}
	return temp.Data.Data[key], nil
}
