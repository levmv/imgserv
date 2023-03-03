package main

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
)

type VerifySignatureFunc func(string) (string, error)

type UrlSignature struct {
	Secret string
	Verify VerifySignatureFunc
}

func NewUrlSignature(method string, secret string) UrlSignature {
	sign := UrlSignature{
		Secret: secret,
	}
	switch method {
	case "st3":
		sign.Verify = ST3sign
	case "t3":
		sign.Verify = sign.T3sign
	default:
		sign.Verify = none
	}
	return sign
}

func none(path string) (string, error) {
	return path, nil
}

// ST3sign used for legacy signatures as first part of path. Just dropping that part (it's already verified by nginx)
func ST3sign(path string) (string, error) {
	if len(path) < 25 {
		return path, fmt.Errorf("too short path: %s", path)
	}

	return path[25:], nil
}

func (sig UrlSignature) T3sign(path string) (string, error) {
	return shortHash(path, sig.Secret, 8, 3), nil
}

func shortHash(str string, secret string, offset int, size int) string {
	hash := md5.Sum([]byte(str + secret))
	return base64.RawURLEncoding.EncodeToString(hash[offset:size])
}
