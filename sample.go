package main

import "net/http"

var data = []byte{
}

type User struct {
}

func sample() []byte {
	var method string
	if method == http.MethodGet {
		return data
	}
	return nil
}

func (u *User) BillingAddress() string {
	return ""
}
