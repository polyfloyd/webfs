package main

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/sessions"

	"webfs/src/fs"
)

var rePasswd = regexp.MustCompile("(?m)^([^\\s]+)\\s([^\\s]+)$")

// Finds the password file by recursively looking in the parent directories of
// the specified file until the root of the virtual filesystem is reached. If
// no password file exists, nil is returned.
func findAuthFile(filesystem *fs.Filesystem, filename string) (string, error) {
	dir := filename
	if info, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("could not find auth file: %v", err)
	} else if !info.IsDir() {
		dir = filepath.Dir(filename)
	}

	for strings.HasPrefix(dir, filesystem.Mount()) {
		f := filepath.Join(dir, ".passwd.txt")
		if _, err := os.Stat(f); err == nil {
			return f, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("could not find auth file: %v", err)
		}
		dir = filepath.Dir(dir)
	}
	return "", nil
}

func authFileAuthenticate(authFile string, rUsername, rPassword string) (bool, error) {
	buf, err := ioutil.ReadFile(authFile)
	if err != nil {
		// Deny access if the password file can not be read.
		return false, fmt.Errorf("error opening password file: %v", err)
	}

	matches := rePasswd.FindAllStringSubmatch(string(buf), -1)
	if matches == nil {
		return false, fmt.Errorf("password file %q is not valid", authFile)
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
	Authenticate(filename string, w http.ResponseWriter, r *http.Request) (bool, error)

	HasPassword(filename string) (bool, error)

	FSAuthenticator(req *http.Request) fs.Authenticator
}

// An Authenticator that just allows everything. Useful for debuging purposes.
type NilAuthenticator struct {
	Filesystem *fs.Filesystem
}

func (NilAuthenticator) Authenticate(string, http.ResponseWriter, *http.Request) (bool, error) {
	return true, nil
}

func (a NilAuthenticator) HasPassword(filename string) (bool, error) {
	authFile, err := findAuthFile(a.Filesystem, filename)
	return authFile != "", err
}

func (NilAuthenticator) FSAuthenticator(req *http.Request) fs.Authenticator {
	return fs.AuthenticatorFunc(func(string) error { return nil })
}

type BasicAuthenticator struct {
	filesystem *fs.Filesystem
	store      sessions.Store
}

func NewBasicAuthenticator(filesystem *fs.Filesystem, storageDir string) (*BasicAuthenticator, error) {
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
		filesystem: filesystem,
		store:      sessions.NewFilesystemStore(storageDir, secret),
	}, nil
}

func (auth *BasicAuthenticator) Authenticate(filename string, w http.ResponseWriter, r *http.Request) (bool, error) {
	// First, look for a .passwd.txt, the file is protected if it is found.
	passwdFile, err := findAuthFile(auth.filesystem, filename)
	if err != nil {
		return false, err
	}
	if passwdFile == "" {
		return true, nil
	}

	// Load the session and check wether the passwd file has been previously unlocked.
	sess, err := auth.store.Get(r, "auth")
	if err != nil {
		sess, err = auth.store.New(r, "auth")
	}
	_, sessAuth := sess.Values[passwdFile]

	// Not authenticated? Check for username and password.
	if !sessAuth {
		time.Sleep(time.Millisecond * 200) // Mitigate brute force attack.

		if rUsername, rPassword, ok := r.BasicAuth(); ok {
			authenticated, err := authFileAuthenticate(passwdFile, rUsername, rPassword)
			if err != nil {
				return false, err
			}

			if authenticated {
				sess.Values[passwdFile] = true
				sess.Save(r, w)
				return true, nil
			}
		}

		// Prompt the user if none found.
		w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"Enter the credentials for %s\"", strings.Replace(filepath.Base(filename), "\"", "\\\"", -1)))
		w.WriteHeader(http.StatusUnauthorized)
		return false, nil
	}

	// The file has been authenticated by the session.
	return true, nil
}

func (auth *BasicAuthenticator) HasPassword(filename string) (bool, error) {
	passwdFile, err := findAuthFile(auth.filesystem, filename)
	return passwdFile != "", err
}

func (auth *BasicAuthenticator) FSAuthenticator(r *http.Request) fs.Authenticator {
	return fs.AuthenticatorFunc(func(filename string) error {
		if filename == "/home/polyfloyd/Projects/webfs/testdata/home/polyfloyd/Projects/webfs/testdata" {
			panic(filename)
		}
		passwdFile, err := findAuthFile(auth.filesystem, filename)
		if err != nil {
			return err
		}
		if passwdFile == "" {
			return nil
		}
		sess, err := auth.store.Get(r, "auth")
		if err != nil {
			return fmt.Errorf("error getting session: %v", err)
		}
		if _, sessAuth := sess.Values[passwdFile]; !sessAuth {
			return fs.ErrNeedAuthentication
		}
		return nil
	})
}
