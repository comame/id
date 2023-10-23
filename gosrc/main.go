package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/comame/accounts.comame.xyz/auth"
	"github.com/comame/accounts.comame.xyz/kvs"
	"github.com/comame/accounts.comame.xyz/oidc"
	"github.com/comame/accounts.comame.xyz/scripts"
	"github.com/comame/mysql-go"
	"github.com/comame/router-go"
)

func init() {
	if err := mysql.Initialize(); err != nil {
		panic(err)
	}

	// TODO: 環境変数から読む
	kvs.InitializeRedis("dev.accounts.comame.xyz", "redis.comame.dev:6379")
}

func main() {
	args := os.Args
	if len(args) >= 2 {
		subcommand := args[1]
		if subcommand != "script" {
			return
		}
		if len(args) < 3 {
			return
		}

		scriptName := args[2]
		scriptArgs := args[3:]
		scripts.Run(scriptName, scriptArgs...)
		return
	}

	// TODO: いずれ消す
	tmpNotFound := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "unimplemented")
	}

	router.Get("/signin", handle_GET_signin)

	router.Get("/authenticate", handle_GET_authenticate)
	router.Post("/authenticate", handle_POST_authenticate)
	router.Post("/code", handle_POST_code)
	router.Get("/userinfo", handle_GET_userinfo)
	router.Post("/userinfo", handle_POST_userinfo)
	router.Get("/.well-known/openid-configuration", handle_GET_wellknownOpenIDConfiguration)
	router.Get("/certs", handle_GET_certs)

	router.Post("/signin/google", tmpNotFound)
	router.Post("/api/signin-password", handle_GET_apiSigninPassword)
	router.Get("/oidc-callback/google", tmpNotFound)

	router.Get("/*", handle_GET_rest)

	router.All("/*", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	log.Println("Start http://localhost:8080")
	http.ListenAndServe(":8080", router.Handler())
}

func handle_GET_signin(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	stateID := q.Get("sid")
	clientID := q.Get("cid")

	if stateID == "" || clientID == "" {
		// TODO: ちゃんとエラー画面を出す
		io.WriteString(w, "err")
		return
	}

	// TODO: 静的に読みたい
	f, err := os.Open("/home/comame/github.com/comame/id/static/front/src/signin.html")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "error")
		return
	}
	defer f.Close()

	io.Copy(w, f)
}

func handle_GET_authenticate(w http.ResponseWriter, r *http.Request) {
	authenticationRequest(w, r.URL.Query())
}

func handle_POST_authenticate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "bad_request" }`)
		return
	}

	authenticationRequest(w, r.Form)
}

func handle_POST_code(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json;charset=UTF-8")
	w.Header().Add("Cache-Control", "no-store")
	w.Header().Add("Pragma", "no-cache")

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "invalid_request" }`)
		return
	}
	req := oidc.ParseCodeRequest(r.Form)

	// client_secret_basic
	clientID, clientSecret, ok := r.BasicAuth()
	if ok && (req.ClientID != "" || req.ClientSecret != "") {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "invalid_request" }`)
		return
	}
	if ok {
		req.ClientID = clientID
		req.ClientSecret = clientSecret
	}

	res, err := oidc.HandleCodeRequest(req)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "invalid_request" }`)
		return
	}

	j, err := json.Marshal(res)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "invalid_request" }`)
		return
	}

	if res.Error != "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(j)
		return
	}

	w.Write(j)
}

func handle_GET_userinfo(w http.ResponseWriter, r *http.Request) {
	userinfoRequest(w, r)
}

func handle_POST_userinfo(w http.ResponseWriter, r *http.Request) {
	userinfoRequest(w, r)
}

type req_GET_apiSigninPassword struct {
	UserId         string `json:"user_id"`
	Password       string `json:"password"`
	CSRFToken      string `json:"csrf_token"`
	RelyingPartyID string `json:"relying_party_id"`
	UserAgentID    string `json:"user_agent_id"`
	StateID        string `json:"state_id"`
}

func handle_GET_apiSigninPassword(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		log.Println("failed to read request body")
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `"{"error": "bad_request" }`)
		return
	}

	var req req_GET_apiSigninPassword
	if err := json.Unmarshal(b, &req); err != nil {
		log.Println("failed to parse json")
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `"{"error": "bad_request" }`)
		return
	}

	passOk, err := auth.AuthenticateByPassword(r.Context(), req.UserId, req.Password, req.RelyingPartyID, req.UserAgentID)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "bad_request" }`)
		return
	}
	if !passOk {
		log.Println("パスワードが違う")
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "invalid_credential" }`)
		return
	}

	roleOk, err := auth.Authorized(req.UserId, req.RelyingPartyID)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "bad_request" }`)
		return
	}
	if !roleOk {
		log.Println("権限がない")
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "unauthorized" }`)
	}

	ar, err := oidc.PostAuthentication(req.UserId, req.StateID, req.RelyingPartyID, req.UserAgentID, auth.AuthenticationMethodPassword)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "bad_request" }`)
		return
	}
	loc, err := oidc.CreateRedirectURLFromAuthenticationResponse(ar)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "bad_request" }`)
		return
	}

	// TODO: 通常のリダイレクトにしたい
	io.WriteString(w, fmt.Sprintf(`{ "location": "%s" }`, loc))
}

func handle_GET_wellknownOpenIDConfiguration(w http.ResponseWriter, r *http.Request) {
	j, err := oidc.GetDiscoveryConfigurationJSON("https://accounts.comame.xyz")
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal Server Error")
		return
	}

	w.Write(j)
}

func handle_GET_certs(w http.ResponseWriter, _ *http.Request) {
	js, err := oidc.GetDiscoveryCertsJSON()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal Server Error")
		return
	}

	w.Write(js)
}

func handle_GET_rest(w http.ResponseWriter, r *http.Request) {
	// TODO: 静的に読むようにしたい
	srv := http.FileServer(http.Dir("/home/comame/github.com/comame/id/static"))
	srv.ServeHTTP(w, r)
}

func authenticationRequest(w http.ResponseWriter, body url.Values) {
	req, err := oidc.ParseAuthenticationRequestFromQuery(body)
	if err != nil {
		log.Println("failed to parse authenticationRequest")
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "bad_request" }`)
		return
	}

	if err := req.Assert(); err != nil {
		log.Println("failed to assert authenticationRequest")
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{ "error": "bad_request" }`)
		return
	}

	id, err := oidc.PreAuthenticate(*req)
	if err != nil {
		log.Println(err)
		perr, ok := err.(oidc.PreAuthenticateError)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{ "error": "bad_request" }`)
			return
		}
		if !perr.NotifyToClient {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{ "error": "bad_request" }`)
			return
		}

		u, err := url.Parse(req.RedirectURI)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{ "error": "bad_request" }`)
			return
		}

		q := u.Query()
		q.Add("error", perr.Error())
		if req.State != "" {
			q.Add("state", req.State)
		}
		u.RawQuery = q.Encode()

		w.Header().Add("Location", u.String())
		w.WriteHeader(http.StatusFound)
		return
	}

	u := fmt.Sprintf("/signin?sid=%s&cid=%s", id, req.ClientId)
	w.Header().Add("Location", u)
	w.WriteHeader(http.StatusFound)
}

func userinfoRequest(w http.ResponseWriter, r *http.Request) {
	var at string

	// RFC6750 2.1
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		if len(authHeader) < len("Bearer a") {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_request"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !strings.HasPrefix(authHeader, "Bearer ") {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_request"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		at = strings.TrimPrefix(authHeader, "Bearer ")
	}

	// RFC6750 2.3
	q := r.URL.Query().Get("access_token")
	if q != "" && at != "" {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_request"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if q != "" {
		at = q
	}

	// RFC6750 2.1
	if at == "" {
		if err := r.ParseForm(); err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_request"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		at = r.Form.Get("access_token")
		if at == "" {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_request"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	j, err := oidc.GetUserinfoJSON(at)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusUnauthorized)
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(j)
}
