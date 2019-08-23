package main

import (
	"flag"
	"fmt"
	"github.com/simonmacklin/kubernetes-vault-webhook/pkg/vault"
	"log"
	"os"
	"strings"
)

type arrayFlags []string

func (i *arrayFlags) String() string {
	return "my string representation"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var (
	myflags arrayFlags
	path    string
	jwt     string
	addr    string
	role    string
)

func init() {
	flag.StringVar(&path, "path", "/var/run/secrets/vault/secrets", "path of the file to write secrets too")
	flag.StringVar(&jwt, "jwt", "/var/run/secrets/kubernetes.io/serviceaccount/token", "path of the file to write secrets too")
	flag.StringVar(&role, "role", "demo", "the vault role to use to grab secrets")
	flag.Var(&myflags, "secret", "the env variable name, vault path and key")
	flag.StringVar(&addr, "address", "http://127.0.0.1:8200", "address of the vault server")
	flag.Parse()
}

func main() {
	kv, err := parseKeyValues(myflags)
	if err != nil {
		log.Fatalf("error parsing cmd line flags: %s", err)
	}

	s, err := getSecrets(kv)
	if err != nil {
		log.Fatalf("failed to retrieve secrets %s", err)
	}

	if err := writeSecrets(path, "kv", s); err != nil {
		log.Fatalf("failed to write secrets to %s: %s", path, err)
	}
}

type mapping struct {
	env  string
	path string
	key  string
}

func parseKeyValues(flags arrayFlags) ([]mapping, error) {
	var m []mapping

	for _, flag := range flags {
		flag = strings.TrimSpace(flag)
		element := strings.Split(flag, ":")
		m = append(m, mapping{
			env:  element[0],
			path: element[1],
			key:  element[2],
		})
	}
	return m, nil
}

func getSecrets(m []mapping) (map[string]string, error) {

	secrets := map[string]string{}

	c, err := vault.NewClient(jwt, addr, role)
	if err != nil {
		return secrets, fmt.Errorf("failed to connect to vault %s", err)
	}
	for _, v := range m {
		s, err := c.GetSecret(v.path, v.key)
		if err != nil {
			return secrets, fmt.Errorf("could not retrieve secret: %s from vault: %s", v.key, err)
		}
		secrets[v.env] = s
	}
	return secrets, nil
}

func writeSecrets(path, format string, secrets map[string]string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	defer f.Close()
	if err != nil {
		return fmt.Errorf("error creating or opening file: %s", err)
	}
	switch format {
	case "kv":
		qq := tokeyValue(secrets)
		if _, err = f.WriteString(qq); err != nil {
			return fmt.Errorf("error writting secrets to path %s : %s", path, err)
		}
		log.Printf("written %v", len(secrets))

	case "json":

	default:
		return fmt.Errorf("%s is not a valid secret format", format)
	}
	return nil
}

func tokeyValue(secrets map[string]string) string {
	sl := make([]string, len(secrets))
	for k, v := range secrets {
		sl = append(sl, fmt.Sprintf("%s=%s\n", k, v))
	}
	return strings.Join(sl, "")
}

func toJson(secrets map[string]string) string {
	return ""
}
