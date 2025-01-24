package main

import (
	"embed"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/comame/accounts.comame.xyz/internal/ceremony"
	"github.com/comame/accounts.comame.xyz/internal/db"
	"github.com/comame/accounts.comame.xyz/internal/kvs"
	"github.com/comame/accounts.comame.xyz/internal/scripts"
	"github.com/comame/router-go"
)

func init() {
	db.Initialize()

	// TODO: 環境変数から読む
	rhost := os.Getenv("REDIS_HOST")
	if rhost == "" {
		panic("REDIS_HOSTが未指定")
	}
	kvs.InitializeRedis("dev.accounts.comame.xyz", rhost)

	log.SetFlags(log.LstdFlags | log.Lshortfile)
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

	log.Println("Start http://localhost:8080")
	http.ListenAndServe(":8080", getAppHandler())
}

func getAppHandler() http.Handler {
	router.Get("/signin", handle_GET_signin)

	router.Get("/authenticate", handle_GET_authenticate)
	router.Post("/authenticate", handle_POST_authenticate)
	router.Post("/code", handle_POST_code)
	router.Get("/userinfo", handle_GET_userinfo)
	router.Post("/userinfo", handle_POST_userinfo)
	router.Get("/.well-known/openid-configuration", handle_GET_wellknownOpenIDConfiguration)
	router.Get("/certs", handle_GET_certs)

	router.Post("/signin/google", handle_POST_signinGoogle)
	router.Post("/api/signin-password", handle_GET_apiSigninPassword)
	router.Get("/oidc-callback/google", handle_GET_oidCallbackGoogle)

	router.Post("/demo/passkey/register-options", handle_Post_passkeyRegisterOptions)
	router.Post("/demo/passkey/register", handle_Post_passkeyRegister)
	router.Post("/demo/passkey/signin-options", handle_Post_passkeySigninOptions)
	router.Post("/demo/passkey/verify", handle_Post_passkeyVerify)

	router.Get("/*", handle_GET_rest)

	router.All("/*", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	return router.Handler()
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

	f, err := getStaticFS().Open("static/front/src/signin.html")
	if err != nil {
		log.Println(err)
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
	ceremony.HandleCodeRequest(w, r)
}

func handle_GET_userinfo(w http.ResponseWriter, r *http.Request) {
	userinfoRequest(w, r)
}

func handle_POST_userinfo(w http.ResponseWriter, r *http.Request) {
	userinfoRequest(w, r)
}

func handle_GET_apiSigninPassword(w http.ResponseWriter, r *http.Request) {
	ceremony.SigninWithPassword(w, r.Body)
}

func handle_POST_signinGoogle(w http.ResponseWriter, r *http.Request) {
	ceremony.StartGoogleSignin(w, r.Body)
}

func handle_GET_oidCallbackGoogle(w http.ResponseWriter, r *http.Request) {
	ceremony.HandleCallbackFromGoogle(w, r)
}

func handle_GET_wellknownOpenIDConfiguration(w http.ResponseWriter, r *http.Request) {
	j, err := ceremony.GetDiscoveryConfigurationJSON("https://accounts.comame.xyz")
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal Server Error")
		return
	}

	w.Write(j)
}

func handle_GET_certs(w http.ResponseWriter, _ *http.Request) {
	js, err := ceremony.GetDiscoveryCertsJSON()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal Server Error")
		return
	}

	w.Write(js)
}

func handle_Post_passkeyRegisterOptions(w http.ResponseWriter, _ *http.Request) {
	ceremony.HandlePasskeyRegistrationOptionsDemo(w)
}

func handle_Post_passkeyRegister(w http.ResponseWriter, r *http.Request) {
	ceremony.HandleRegisterPasskeyDemo(w, r)
}

func handle_Post_passkeySigninOptions(w http.ResponseWriter, _ *http.Request) {
	ceremony.HandlePasskeyGetOptionsDemo(w)
}

func handle_Post_passkeyVerify(w http.ResponseWriter, r *http.Request) {
	ceremony.HandlePasskeyVerify(w, r)
}

func handle_GET_rest(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(getStaticFS(), "static")
	if err != nil {
		panic(err)
	}
	srv := http.FileServer(http.FS(sub))
	srv.ServeHTTP(w, r)
}

func authenticationRequest(w http.ResponseWriter, body url.Values) {
	ceremony.AuthenticationRequest(w, body)
}

func userinfoRequest(w http.ResponseWriter, r *http.Request) {
	ceremony.HandleUserInfoRequest(w, r)
}

//go:embed static
var embedStaticFS embed.FS

func getStaticFS() fs.FS {
	if os.Getenv("DEV") == "" {
		return embedStaticFS
	}

	return os.DirFS(".")
}
