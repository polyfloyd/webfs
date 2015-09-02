package main

import (
	"./fs"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"time"
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
		children = map[string]fs.File{}
	}
	passwd, ok := children[".passwd.txt"]
	if !ok {
		parent := file.Parent()
		if parent == nil {
			return nil, nil
		}
		return findAuthFile(parent)
	}

	return &passwd, nil
}

// Check if a file needs authentication to view.
// Returns true if it's safe to transfer the protected resource to the client.
//
// The password file that is looked for is simply called .passwd.txt and
// contains a list of possible username/password pairs separated by newlines.
// The username and password are separated by whitespace. Neither the username
// or password may therefore contain whitespace.
func IsAuthenticated(file *fs.File, req *http.Request) (bool, error) {
	if file.Path == "" || file.Path == "/" {
		return true, nil
	}

	passwd, err := findAuthFile(file)
	if err != nil {
		return false, fmt.Errorf("Error finding password file: ", err)
	}
	if passwd == nil {
		return true, nil
	}

	fd, err := passwd.Open()
	if err != nil {
		// Deny access if the paswd file can not be read.
		return false, fmt.Errorf("Error opening password file: ", err)
	}

	rUsername, rPassword, ok := req.BasicAuth()
	if !ok {
		return false, nil
	}

	var buf [2048]byte
	n, _ := fd.Read(buf[:])
	matches := passwdMatcher.FindAllStringSubmatch(string(buf[:n]), -1)
	if matches == nil {
		return false, fmt.Errorf("Password file \"%v\" is not valid.", passwd.RealPath())
	}
	for _, match := range matches {
		if match[1] == rUsername && match[2] == rPassword {
			return true, nil
		}
	}

	return false, nil
}

func Authenticate(file *fs.File, res http.ResponseWriter, req *http.Request) bool {
	authenticated, err := IsAuthenticated(file, req)
	if err != nil {
		log.Println(err)
	}
	if !authenticated {
		time.Sleep(time.Millisecond * 200) // Mitigate brute force attack.
		res.Header().Set("WWW-Authenticate", "Basic realm=\"You need a username and password to view these files\"")
		http.Error(res, "Unauthorized", http.StatusUnauthorized)
	}
	return authenticated
}
