package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	//	"strings"
	"time"
)

var serverStart time.Time
var apiClient *http.Client

var templates = template.Must(template.ParseFiles("./templates/main.html"))
var linksPattern = regexp.MustCompile("\\<(https\\://api\\.github\\.com/user/repos(\\?page=[0-9]+.*?))\\>; rel=\"(first|next|prev|last)\"")

func init() {
	scrts, err := getSecrets()
	if err != nil {
		fmt.Printf("egads!")
		return
	}
	secrets = &scrts

	serverStart = time.Now()
	apiClient = &http.Client{}
}

var secrets *AppData

type AppData struct {
	Id             string `json:"client_id"`
	Secret         string `json:"client_secret"`
	CallbackUrl    string `json:"callback_url"`
	Scope          string `json:"scope"`
	OauthUrl       string `json:"oauth_url"`
	AccessTokenUrl string `json:"access_token_url"`
	ApiUrl         string `json:"api_base_url"`
}

type AccessToken struct {
	Scope string
	Token string
	Type  string
	Url   string
}

type Project struct {
	Id       int    `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
}

func (data AppData) GetOauthUrl() string {
	elapsedDuration := time.Since(serverStart).Nanoseconds()
	oauthUrl := fmt.Sprintf(data.OauthUrl, data.Id, data.CallbackUrl, data.Scope, elapsedDuration)
	return oauthUrl
}

func (data AppData) GetAccessUrl(code string) string {
	accessUrl := fmt.Sprintf(data.AccessTokenUrl, data.Id, data.Secret, code)
	return accessUrl
}

func parseLinks(linkString string) map[string]string {
	links := linksPattern.FindAllStringSubmatch(linkString, -1)
	mappedLinks := make(map[string]string)
	for _, row := range links {
		link := row[1]
		link_type := row[3]
		mappedLinks[link_type] = link
	}
	return mappedLinks
}

func GetRepositoriesPage(requestUrl string, authHeader string) []Project {
	req, err := http.NewRequest("GET", requestUrl, nil)
	req.Header.Add("Authorization", authHeader)
	if err != nil {
		fmt.Printf("panic!")
	}
	resp, err := apiClient.Do(req)

	defer resp.Body.Close()
	var rows []Project

	err = json.NewDecoder(resp.Body).Decode(&rows)

	// <https://api.github.com/user/repos?page=2>; rel="next", <https://api.github.com/user/repos?page=4>; rel="last"
	links := resp.Header.Get("Link")
	if links == "" {
		return rows
	}

	parsedLinks := parseLinks(links)
	nextUrl, exists := parsedLinks["next"]
	if exists {
		rows = append(rows, GetRepositoriesPage(nextUrl, authHeader)...)
	}
	fmt.Printf("%d - ", len(rows))
	return rows
}

func (accessToken AccessToken) GetPublicRepositories() []Project {
	requestUrl := accessToken.Url + "user/repos"

	authHeader := fmt.Sprintf("%s %s", accessToken.Type, accessToken.Token)
	rows := GetRepositoriesPage(requestUrl, authHeader)
	return rows
}

func getSecrets() (AppData, error) {
	fileData, err := os.Open("./secrets/app_secrets.json")
	if err != nil {
		fmt.Printf("secrets not properly set.")
		return AppData{}, err
	}
	defer fileData.Close()

	var data AppData
	err = json.NewDecoder(fileData).Decode(&data)
	if err != nil {
		fmt.Printf("Ill formatted JSON.")
		return AppData{}, err
	}

	return data, nil
}

func viewHandler(respWriter http.ResponseWriter, request *http.Request) {
	log.Println("helloView.")
	err := templates.ExecuteTemplate(respWriter, "main.html", secrets)
	if err != nil {
		http.Error(respWriter, err.Error(), http.StatusInternalServerError)
		return
	}
}

func isStaleRequest(stateTokens []string) bool {
	state, err := time.ParseDuration(stateTokens[0] + "ns")
	if err != nil {
		fmt.Printf("Incorrect state format.")
		return false
	}
	flowStart := state.Minutes()
	flowEnd := time.Since(serverStart).Minutes()

	flowDuration := flowEnd - flowStart
	return flowDuration < 0 || flowDuration > 2
}

func requestAccessToken(codeTokens []string) (map[string][]string, error) {
	code := codeTokens[0]
	requestUrl := secrets.GetAccessUrl(code)
	tokenResponse, err := http.Get(requestUrl)
	if err != nil {
		fmt.Printf("bad token request.")
		return nil, err
	}
	tokenBytes, err := ioutil.ReadAll(tokenResponse.Body)
	if err != nil {
		fmt.Printf("Bad Token response.")
		return nil, err
	}
	defer tokenResponse.Body.Close()
	tokenData, err := url.ParseQuery(string(tokenBytes))
	hasFailed := tokenData["error"]
	if hasFailed != nil {
		fmt.Printf("Danger will robinson!\n")
		return nil, errors.New("Boo@")
	}
	return tokenData, nil
}

func callbackHandler(respWriter http.ResponseWriter, request *http.Request) {
	Values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		fmt.Printf("err")
		return
	}
	if isStaleRequest(Values["state"]) {
		http.Error(respWriter, "Timeout: Please try again.", http.StatusRequestTimeout)
		return
	}

	tokenData, err := requestAccessToken(Values["code"])
	if err != nil {
		fmt.Printf("Danger Will Robinson!\n")
		return
	}

	token := AccessToken{
		Token: tokenData["access_token"][0],
		Type:  tokenData["token_type"][0],
		Url:   secrets.ApiUrl,
	}
	token.GetPublicRepositories()
}

func main() {
	http.Handle("/vendor/", http.FileServer(http.Dir(".")))
	http.HandleFunc("/callback", callbackHandler)
	http.HandleFunc("/", viewHandler)

	log.Println("Starting server... http://localhost:8080/")
	http.ListenAndServe(":8080", nil)
	log.Println("Server terminated")
}
