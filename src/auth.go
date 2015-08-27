package main

import (
	"./fs"
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

func denyAccess(res http.ResponseWriter) {
	time.Sleep(time.Millisecond * 200) // Mitigate brute force attack.
	res.Header().Set("WWW-Authenticate", "Basic realm=\"You need a username and password to view these files\"")
	http.Error(res, "Unauthorized", http.StatusUnauthorized)
}

// Check if a file needs authentication to view.
// Returns true if it's safe to transfer the protected resource to the client.
//
// The password file that is looked for is simply called .passwd.txt and
// contains a list of possible username/password pairs separated by newlines.
// The username and password are separated by whitespace. Neither the username
// or password may therefore contain whitespace.
func Authenticate(file *fs.File, res http.ResponseWriter, req *http.Request) bool {
	passwd, err := findAuthFile(file)
	if err != nil {
		log.Println("Error finding password file: ", err)
		denyAccess(res)
		return false
	}
	if passwd == nil {
		return true
	}

	fd, err := passwd.Open()
	if err != nil {
		log.Println("Error opening password file: ", err)
		denyAccess(res)
		return false // Deny access if the paswd file can not be read.
	}

	rUsername, rPassword, ok := req.BasicAuth()
	if !ok {
		denyAccess(res)
		return false
	}

	var buf [2048]byte
	n, _ := fd.Read(buf[:])
	matches := passwdMatcher.FindAllStringSubmatch(string(buf[:n]), -1)
	if matches == nil {
		log.Printf("Password file \"%v\" is not valid.", passwd.RealPath())
		denyAccess(res)
		return false
	}
	for _, match := range matches {
		if match[1] == rUsername && match[2] == rPassword {
			return true
		}
	}

	denyAccess(res)
	return false
}
