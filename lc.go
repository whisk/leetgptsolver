package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"slices"
	"time"

	cloudflarebp "github.com/DaRealFreak/cloudflare-bp-go"
	browser "github.com/EDDYCJY/fake-useragent"
	"github.com/browserutils/kooky"
	_ "github.com/browserutils/kooky/browser/chrome"
	_ "github.com/browserutils/kooky/browser/firefox"
	"github.com/rs/zerolog/log"
)

type UgcArticleSolutionArticles struct {
	Data struct {
		UgcArticleSolutionArticles struct {
			TotalNum int `json:"totalNum"`
			Edges    []struct {
				Node struct {
					CreatedAt string `json:"createdAt"`
				}
			}
		}
	}
}

type DiscussionTopic struct {
	Data struct {
		QuestionDiscussionTopic struct {
			Id                   int
			CommentCount         int
			TopLevelCommentCount int
		}
	}
}

type QuestionDiscussCommentsResp struct {
	Data struct {
		TopicComments struct {
			Data []struct {
				Id   int
				Post struct {
					Id           int
					CreationDate int
				}
			}
			TotalNum int
		}
	}
}

var cookieJarCache *cookiejar.Jar
var leetcodeUrl *url.URL
var leetcodeGraphqlUrl *url.URL

func init() {
	u, err := url.Parse("https://leetcode.com/")
	if err != nil {
		panic(fmt.Sprintf("failed to parse leetcode url: %v. This is a bug", err))
	}
	leetcodeUrl = u
	leetcodeGraphqlUrl = &url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   "/graphql",
	}
}

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
	log.Trace().Msgf("%s %s", method, url)
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

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
	log.Trace().Msgf("http response %s", resp.Status)

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response body: %w", err)
	}
	log.Trace().Msgf("got %d bytes body", len(respBody))
	if resp.StatusCode != http.StatusOK {
		return respBody, resp.StatusCode, fmt.Errorf("non-ok http response code: %d", resp.StatusCode)
	}
	return respBody, resp.StatusCode, nil
}

func cookieJar() http.CookieJar {
	// nil means cookies was never loaded
	if cookieJarCache != nil {
		log.Trace().Msg("using cached cookie jar")
		return cookieJarCache
	}
	loadedJar, err := loadCookieJar()
	if err != nil {
		// if loading of cookies failed, we don't want to make more unsuccessful attempts
		// instead we log the error and cache an empty jar (note that empty is not nil)
		log.Err(err).Msg("failed to get the cookie jar")
		cookieJarCache, _ = cookiejar.New(nil)
		return cookieJarCache
	}
	loadedJarTyped, ok := loadedJar.(*cookiejar.Jar)
	if !ok {
		log.Err(err).Msg("loaded cookie jar is not a cookie jar (this is a bug)")
		cookieJarCache, _ = cookiejar.New(nil)
	}
	cookieJarCache = loadedJarTyped

	return cookieJarCache
}

func loadCookieJar() (http.CookieJar, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to read chrome cookies: %w", err)
	}
	cookiesFile := dir + "/Google/Chrome/Default/Cookies"
	if ok, _ := fileExists(cookiesFile); !ok {
		return nil, fmt.Errorf("chrome cookies file not found or invalid: %s", cookiesFile)
	}

	for _, cookieStore := range kooky.FindAllCookieStores() {
		if ok, _ := fileExists(cookieStore.FilePath()); !ok {
			// skip non-existing cookie stores
			continue
		}
		log.Trace().Msgf("Found cookie store for %s: %s (default: %v)", cookieStore.Browser(), cookieStore.FilePath(), cookieStore.IsDefaultProfile())
		if cookieStore.Browser() == "firefox" && cookieStore.IsDefaultProfile() {
			// modern chrome cookies are not supported by kooky
			subJar, err := cookieStore.SubJar(kooky.Valid)
			defer cookieStore.Close()

			if err != nil {
				log.Err(err).Msgf("failed to get cookie sub jar from %s", cookieStore.FilePath())
				continue
			}
			log.Debug().Msgf("using cookie jar from %s", cookieStore.FilePath())
			return subJar, nil
		}
	}

	return nil, fmt.Errorf("no cookie stores found")
}

func cookie(name string) (*http.Cookie, error) {
	jar := cookieJar()
	cookies := jar.Cookies(leetcodeUrl)
	idx := slices.IndexFunc(cookies, func(c *http.Cookie) bool { return c.Name == name })
	if idx == -1 {
		return nil, nil
	}
	return cookies[idx], nil
}

func newHeader() http.Header {
	cookie, _ := cookie("csrftoken")
	token := ""
	if cookie != nil {
		token = cookie.Value
	}

	return http.Header{
		"Content-Type": {"application/json"},
		"User-Agent":   {browser.Firefox()},
		"X-Csrftoken":  {token},
	}
}

func client() *http.Client {
	client := http.DefaultClient
	client.Transport = newTransport()
	client.Jar = cookieJar()

	return client
}

// &http.Transport{} bypasses cloudflare generally better than DefaultTransport
func newTransport() http.RoundTripper {
	return cloudflarebp.AddCloudFlareByPass(&http.Transport{})
}

func SubmitUrl(q Question) string {
	return leetcodeUrl.Scheme + "://" + leetcodeUrl.Host + "/problems/" + q.Data.Question.TitleSlug + "/submit/"
}

func SubmissionCheckUrl(submissionId uint64) string {
	return leetcodeUrl.Scheme + "://" + leetcodeUrl.Host + "/submissions/detail/" + fmt.Sprint(submissionId) + "/check/"
}

// not used, but may be useful in the future
func DiscussionTopicQuery(slug string) ([]byte, error) {
	query := map[string]interface{}{
		"query": `query discussionTopic($questionSlug: String!)
		{
			questionDiscussionTopic(questionSlug: $questionSlug) {
				id
				commentCount
				topLevelCommentCount
			}
		}`,
		"variables": map[string]string{
			"questionSlug": slug,
		},
		"operationName": "discussionTopic",
	}
	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed marshalling GraphQL: %w", err)
	}
	return queryBytes, nil
}

func QuestionDiscussCommentsQuery(topicId, first, pageNo int) ([]byte, error) {
	query := map[string]interface{}{
		"query": `query questionDiscussComments($topicId: Int!, $orderBy: String = "newest_to_oldest", $pageNo: Int = 1, $numPerPage: Int = 10)
		{
			topicComments(
				topicId: $topicId
    			orderBy: $orderBy
    			pageNo: $pageNo
    			numPerPage: $numPerPage
			) {
				data {
					id
					post {
						id
						creationDate
					}
				}
				totalNum
			}
		}`,
		"variables": map[string]string{
			"numPerPage": fmt.Sprint(first),
			"orderBy":    "newest_to_oldest",
			"pageNo":     fmt.Sprint(pageNo),
			"topicId":    fmt.Sprint(topicId),
		},
		"operationName": "questionDiscussComments",
	}
	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed marshalling GraphQL: %w", err)
	}
	return queryBytes, nil
}

func LoadQuestionDiscussComments(slug string, first, pageNo int) (QuestionDiscussCommentsResp, error) {
	var resp QuestionDiscussCommentsResp
	queryBytes, err := DiscussionTopicQuery(slug)
	if err != nil {
		return resp, fmt.Errorf("failed to create query to get discussion topic: %w", err)
	}
	respBody, _, err := makeAuthorizedHttpRequest("POST", leetcodeGraphqlUrl.String(), bytes.NewReader(queryBytes))
	if err != nil {
		return resp, fmt.Errorf("failed to get discussion topic: %w", err)
	}
	log.Trace().Msgf("got response body: %s", respBody)

	var topicResp DiscussionTopic
	if err := json.Unmarshal(respBody, &topicResp); err != nil {
		return resp, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	topicId := topicResp.Data.QuestionDiscussionTopic.Id
	if topicId <= 0 {
		return resp, fmt.Errorf("invalid or empty topicId for %s: %d", slug, topicId)
	}

	queryBytes, err = QuestionDiscussCommentsQuery(topicId, first, pageNo)
	if err != nil {
		return resp, fmt.Errorf("failed to create query to get discussion comments: %w", err)
	}
	respBody, _, err = makeAuthorizedHttpRequest("POST", leetcodeGraphqlUrl.String(), bytes.NewReader(queryBytes))
	if err != nil {
		return resp, fmt.Errorf("failed to get discussion comments: %w", err)
	}
	log.Trace().Msgf("got response body: %s", respBody)
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return resp, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return resp, nil
}

func UgcArticlesSolutionQuery(slug string, first, skip int) ([]byte, error) {
	query := map[string]interface{}{
		"query": `query ugcArticleSolutionArticles($questionSlug: String!, $orderBy: ArticleOrderByEnum, $userInput: String, $tagSlugs: [String!], $skip: Int, $before: String, $after: String, $first: Int, $last: Int, $isMine: Boolean)
		{
			ugcArticleSolutionArticles(
				questionSlug: $questionSlug
				orderBy: $orderBy
				userInput: $userInput
				tagSlugs: $tagSlugs
				skip: $skip
				first: $first
				before: $before
				after: $after
				last: $last
				isMine: $isMine
			)
			{
			    totalNum
				pageInfo {
					hasNextPage
				}
				edges {
					node {
						...ugcSolutionArticleFragment
					}
				}
			}
		}
		fragment ugcSolutionArticleFragment on SolutionArticleNode {
			uuid
			title
			slug
			summary
			author {
				realName
				userAvatar
				userSlug
				userName
				nameColor
				certificationLevel
				activeBadge {
					icon
					displayName
				}
			}
			articleType
			thumbnail
			summary
			createdAt
			updatedAt
			status
			isLeetcode
			canSee
			canEdit
			isMyFavorite
			chargeType
			myReactionType
			topicId
			hitCount
			hasVideoArticle
			reactions {
				count
				reactionType
			}
			title
			slug
			tags {
				name
				slug
				tagType
			}
			topic {
				id
				topLevelCommentCount
			}
		}`,
		"variables": map[string]interface{}{
			"first":        first,
			"orderBy":      "MOST_RECENT",
			"questionSlug": slug,
			"skip":         skip,
			"tagSlugs":     []string{},
			"userInput":    "",
		},
		"operationName": "ugcArticleSolutionArticles",
	}
	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed marshalling GraphQL: %w", err)
	}
	return queryBytes, nil
}

func LoadSolutions(slug string, perPage, skip int) (UgcArticleSolutionArticles, error) {
	var resp UgcArticleSolutionArticles

	queryBytes, err := UgcArticlesSolutionQuery(slug, perPage, skip)
	if err != nil {
		return resp, fmt.Errorf("failed to create query to get ugc solutions: %w", err)
	}
	log.Trace().Msgf("query to get ugc solutions: %s", string(queryBytes))
	respBody, _, err := makeAuthorizedHttpRequest("POST", leetcodeGraphqlUrl.String(), bytes.NewReader(queryBytes))
	if err != nil {
		return resp, fmt.Errorf("failed to get solutions: %w", err)
	}
	log.Trace().Msgf("got response body: %s", respBody)

	if err := json.Unmarshal(respBody, &resp); err != nil {
		return resp, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return resp, nil
}

// LoadFirstUgcContentTime loads the first (oldest) comment or solution for a given question and extracts its creation time.
func LoadFirstUgcContentTime(slug string) (time.Time, error) {
	perPage := 10

	comments, err := LoadQuestionDiscussComments(slug, perPage, 1)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to load discussion comments: %w", err)
	}
	if comments.Data.TopicComments.TotalNum > perPage {
		log.Debug().Msgf("more than %d comments found for %s, loading the oldest comments...", perPage, slug)
		comments, err = LoadQuestionDiscussComments(slug, perPage, (comments.Data.TopicComments.TotalNum-1)/perPage+1)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to load the oldest comments: %w", err)
		}
	}
	var firstCommentTime time.Time
	if len(comments.Data.TopicComments.Data) > 0 {
		creationDateUnix := comments.Data.TopicComments.Data[len(comments.Data.TopicComments.Data)-1].Post.CreationDate
		if creationDateUnix <= 0 {
			log.Error().Msgf("invalid creation date for %s: %d", slug, creationDateUnix)
		} else {
			firstCommentTime = time.Unix(int64(creationDateUnix), 0)
			log.Debug().Msgf("first comment time for %s: %s", slug, firstCommentTime)
		}
	}

	resp, err := LoadSolutions(slug, perPage, 0)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to load most recent solutions: %w", err)
	}

	var firstSolutionTime time.Time
	if resp.Data.UgcArticleSolutionArticles.TotalNum == 3000 {
		log.Debug().Msgf("likely %s hit the number of solutions limit. Unable to determine the creation time from solutions", slug)
	} else {
		if resp.Data.UgcArticleSolutionArticles.TotalNum > perPage {
			log.Debug().Msgf("more than %d solutions found for %s, loading the oldest solutions...", perPage, slug)
			resp, err = LoadSolutions(slug, perPage, resp.Data.UgcArticleSolutionArticles.TotalNum-perPage)
			if err != nil {
				return time.Time{}, fmt.Errorf("failed to load oldest solutions: %w", err)
			}
		}
		if len(resp.Data.UgcArticleSolutionArticles.Edges) > 0 {
			firstSolutionTimeStr := resp.Data.UgcArticleSolutionArticles.Edges[len(resp.Data.UgcArticleSolutionArticles.Edges)-1].Node.CreatedAt
			firstSolutionTime, err = time.Parse(time.RFC3339Nano, firstSolutionTimeStr)
			if err != nil {
				log.Err(err).Msgf("failed to parse first solution time for %s", slug)
			} else {
				log.Debug().Msgf("first solution time for %s: %s", slug, firstSolutionTime)
			}
		}
	}

	if firstCommentTime.IsZero() {
		return firstSolutionTime, nil
	} else if firstSolutionTime.IsZero() {
		return firstCommentTime, nil
	} else if firstCommentTime.Before(firstSolutionTime) {
		return firstCommentTime, nil
	}

	return firstSolutionTime, nil
}
