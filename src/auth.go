package main

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"./fs"
	"github.com/gorilla/sessions"
)

var passwdMatcher = regexp.MustCompile("(?m)^([^\\s]+)\\s([^\\s]+)$")

// Finds the password file by recursively looking in the parent directories of
// the specified file until the root of the virtual filesystem is reached. If
// no password file exists, nil is returned.
func findAuthFile(file *fs.File) (*fs.File, error) {
	children, err := file.Children()
	if err != nil {
		return nil, err
	}
	if children == nil {
		children = map[string]*fs.File{}
	}

	passwd, ok := children[".passwd.txt"]
	if !ok {
		parent := file.Parent()
		if parent == nil {
			return nil, nil
		}
		return findAuthFile(parent)
	}
	return passwd, nil
}

func authFileAuthenticate(authFile *fs.File, rUsername, rPassword string) (bool, error) {
	buf, err := ioutil.ReadFile(authFile.RealPath())
	if err != nil {
		// Deny access if the password file can not be read.
		return false, fmt.Errorf("Error opening password file: %v", err)
	}

	matches := passwdMatcher.FindAllStringSubmatch(string(buf), -1)
	if matches == nil {
		return false, fmt.Errorf("Password file %q is not valid.", authFile.RealPath())
	}
	for _, match := range matches {
		if match[1] == rUsername && match[2] == rPassword {
			return true, nil
		}
	}

	return false, nil
}

type Authenticator interface {
	// Check if a file needs authentication to view.
	// Returns true if it's safe to transfer the protected resource to the client.
	//
	// The password file that is looked for is simply called .passwd.txt and
	// contains a list of possible username/password pairs separated by newlines.
	// The username and password are separated by whitespace. Neither the username
	// or password may therefore contain whitespace.
	Authenticate(file *fs.File, res http.ResponseWriter, req *http.Request) (bool, error)
}

// An Authenticator that just allows everything. Useful for debuging purposes.
type NilAuthenticator struct{}

func (NilAuthenticator) Authenticate(*fs.File, http.ResponseWriter, *http.Request) (bool, error) {
	return true, nil
}

type BasicAuthenticator struct {
	store sessions.Store
}

func NewBasicAuthenticator(storageDir string) (*BasicAuthenticator, error) {
	storageDir = fs.FixHome(storageDir)
	if err := os.MkdirAll(storageDir, 0700); err != nil {
		return nil, err
	}

	var secret []byte
	secretPath := path.Join(storageDir, "secret")
	if sec, err := ioutil.ReadFile(secretPath); os.IsNotExist(err) {
		secret = make([]byte, 128)
		if _, err := rand.Read(secret); err != nil {
			return nil, err
		}
		if err := ioutil.WriteFile(secretPath, secret, 0700); err != nil {
			return nil, err
		}
	} else if err == nil {
		secret = sec
	} else {
		return nil, err
	}

	return &BasicAuthenticator{
		store: sessions.NewFilesystemStore(storageDir, secret),
	}, nil
}

func (auth *BasicAuthenticator) Authenticate(file *fs.File, res http.ResponseWriter, req *http.Request) (bool, error) {
	// First, look for a .passwd.txt, the file is protected if it is found.
	passwdFile, err := findAuthFile(file)
	if err != nil {
		return false, fmt.Errorf("Error finding password file: %v", err)
	}
	if passwdFile == nil {
		return true, nil
	}

	// Load the session and check wether the passwd file has been previously unlocked.
	sess, err := auth.store.Get(req, "auth")
	if err != nil {
		return false, fmt.Errorf("Error getting session: %v", err)
	}
	_, sessAuth := sess.Values[passwdFile.RealPath()]

	// Not authenticated? Check for username and password.
	if !sessAuth {
		time.Sleep(time.Millisecond * 200) // Mitigate brute force attack.

		if rUsername, rPassword, ok := req.BasicAuth(); ok {
			authenticated, err := authFileAuthenticate(passwdFile, rUsername, rPassword)
			if err != nil {
				return false, err
			}

			if authenticated {
				sess.Values[file.RealPath()] = true
				sess.Save(req, res)
				return true, nil
			}
		}

		// Prompt the user if none found.
		res.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"Enter the credentials for %v\"", strings.Replace(file.Path, "\"", "\\\"", -1)))
		res.WriteHeader(http.StatusUnauthorized)
		return false, nil
	}

	// The file has been authenticated by the session.
	return true, nil
}
