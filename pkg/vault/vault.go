package vault

import (
	//"io"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

type Client struct {
	token   string
	address string
}

func readJwt(path string) (string, error) {
	f, err := ioutil.ReadFile(path)
	if err != nil {
		return "", errors.New(fmt.Sprintf("failed to read file: %s", err))
	}
	return string(bytes.TrimSpace(f)), nil
}

func NewClient(path, address, role string) (*Client, error) {
	c := Client{}
	c.address = address

	jwt, err := readJwt(path)
	if err != nil {
		log.Fatalf("could not read jwt token: %s", err)
	}

	h := &http.Client{}
	body := fmt.Sprintf(`{"role": "%s", "jwt": "%s"}`, role, jwt)
	req, err := http.NewRequest(http.MethodPost, address+"/v1/auth/kubernetes/login", strings.NewReader(strings.TrimSpace(strings.Trim(body, "\n"))))

	

	if err != nil {
		log.Fatalf("failed to make a request to vault: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	// if resp.StatusCode != 200 {
	// 	var b bytes.Buffer
	// 	if _, err := io.Copy(&b, resp.Body); err != nil {
	// 		log.Printf("failed to copy response body: %s", err)
	// 	}
	// 	return "", fmt.Errorf("failed to get successful response: %#v, %s", resp, b.String())
	// }
	
	temp := struct {
		Auth struct {
			Token string `json:"client_token"`
		}
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&temp); err != nil {
		log.Fatalf("error making request to vault: %s", err)
	}

	c.token = temp.Auth.Token

	if c.token == "" {
		return &c, errors.New("error getting vault token")
	}
	return &c, nil
}

func (c *Client) GetSecret(path, key string) (string, error) {
	h := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, c.address+"/v1/secret/data"+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vault-Token", c.token)
	resp, err := h.Do(req)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	temp := struct {
		Data struct {
			Data map[string]string `json:"data"`
		}
	}{}

	if err := json.Unmarshal(body, &temp); err != nil {
		return "", err
	}
	if len(temp.Data.Data) < 1 {
		return "", errors.New(fmt.Sprintf("no secret found with vault path %s", path))
	}

	if _, ok := temp.Data.Data[key]; !ok {
		return "", errors.New(fmt.Sprintf("vault path %s is valid but no key %s exist under that path", key, path))
	}

	return temp.Data.Data[key], nil
}
