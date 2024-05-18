package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"slices"

	cloudflarebp "github.com/DaRealFreak/cloudflare-bp-go"
	browser "github.com/EDDYCJY/fake-useragent"
	"github.com/browserutils/kooky/browser/chrome"
	"github.com/rs/zerolog/log"
)

var cookieJarCache *cookiejar.Jar

func makeNiceReferer(urlStr string) (string, error) {
	url, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	// remove the last fragment of the path
	url.Path = path.Dir(path.Dir(url.Path + "/"))
	return url.String(), nil
}

func makeAuthorizedHttpRequest(method string, url string, reqBody io.Reader) ([]byte, int, error) {
	log.Debug().Msgf("%s %s", method, url)
	req, err := newRequest(method, url, reqBody)

	c := client()
	req.Header = newHeader()
	if referer, err := makeNiceReferer(url); err != nil {
		log.Err(err).Msg("failed to make a referer")
	} else {
		req.Header.Add("Referer", referer)
	}
	req.Header.Add("Content-Length", fmt.Sprintf("%d", req.ContentLength))

	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to do the request: %w", err)
	}
	log.Debug().Msg(resp.Status)

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response body: %w", err)
	}
	log.Debug().Msgf("Got %d bytes body", len(respBody))
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("got HTTP response %d", resp.StatusCode)
	}
	return respBody, resp.StatusCode, nil
}

func cookieJar() http.CookieJar {
	// nil means cookies was never loaded
	if cookieJarCache != nil {
		return cookieJarCache
	}
	loadedJar, err := loadCookieJar()
	if err != nil {
		// if loading of cookies failed, we don't want to make more unsuccessful attempts
		// instead we log the error and cache an empty jar (note that empty is not nil)
		log.Err(err).Msg("failed to get the cookie jar")
		emptyJar, _ := cookiejar.New(nil)
		cookieJarCache = emptyJar
		return emptyJar
	}

	return loadedJar
}

func loadCookieJar() (http.CookieJar, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to read chrome cookies: %w", err)
	}
	cookiesFile := dir + "/Google/Chrome/Default/Cookies"
	cookieJar, err := chrome.CookieJar(cookiesFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read chrome cookies: %w", err)
	}
	return cookieJar, nil
}

func cookie(name string) (*http.Cookie, error) {
	jar := cookieJar()
	url, err := url.Parse("https://leetcode.com/")
	if err != nil {
		return nil, fmt.Errorf("failed to parse cookie domain: %w", err)
	}
	cookies := jar.Cookies(url)
	idx := slices.IndexFunc(cookies, func(c *http.Cookie) bool { return c.Name == name })
	if idx == -1 {
		return nil, nil
	}
	return cookies[idx], nil
}

func newRequest(method string, url string, reqBody io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func newHeader() http.Header {
	cookie, _ := cookie("csrftoken")
	token := ""
	if cookie != nil {
		token = cookie.Value
	}

	return http.Header{
		"Content-Type": {"application/json"},
		"User-Agent":   {browser.Chrome()},
		"X-Csrftoken":  {token},
	}
}

func client() *http.Client {
	client := http.DefaultClient
	client.Transport = newTransport()
	client.Jar, _ = loadCookieJar()

	return client
}

// &http.Transport{} bypasses cloudflare generally better than DefaultTransport
func newTransport() http.RoundTripper {
	return cloudflarebp.AddCloudFlareByPass(&http.Transport{})
}
