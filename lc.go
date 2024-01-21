package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func makeNiceReferer(urlStr string) (string, error) {
	url, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	// remove the last fragment of the path
	url.Path = path.Dir(path.Dir(url.Path + "/"))
	return url.String(), nil
}

func makeAuthorizedHttpRequest(method string, url string, reqBody io.Reader) ([]byte, error) {
	log.Debug().Msgf("%s %s", method, url)
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	c := http.DefaultClient
	req.Header = getHeader()
	if referer, err := makeNiceReferer(url); err != nil {
		log.Err(err).Msg("failed to make a referer")
	} else {
		req.Header.Add("Referer", referer)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do the request: %w", err)
	}
	log.Debug().Msg(resp.Status)

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	log.Debug().Msgf("Got %d bytes body", len(respBody))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("got HTTP response %d", resp.StatusCode)
	}
	return respBody, nil
}

func getHeader() http.Header {
	token := viper.GetString("leetcode_csrf_token")
	session := viper.GetString("leetcode_session")
	return http.Header{
		"Content-Type": {"application/json"},
		"User-Agent": {HTTP_USER_AGENT},
		"Referrer-Policy": {"strict-origin-when-cross-origin"},
		"Cookie": {"LEETCODE_SESSION=" + session + "; csrftoken=" + token + "; "},
		"X-Csrftoken": {token},
	}
}