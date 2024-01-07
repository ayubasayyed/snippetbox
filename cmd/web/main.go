package main

import (
	"crypto/tls"
	"database/sql"
	"flag"
	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/v2"
	"github.com/go-playground/form/v4"
	_ "github.com/go-sql-driver/mysql"
	"github.com/julienschmidt/httprouter"
	"github.com/justinas/alice"
	"html/template"
	"log"
	"net/http"
	"os"
	"snippetbox/internal/models"
	"time"
)

type application struct {
	infoLog        *log.Logger
	errorLog       *log.Logger
	snippets       *models.SnippetModel
	user           *models.UserModel
	templateCache  map[string]*template.Template
	formDecoder    *form.Decoder
	sessionManager *scs.SessionManager
}

func main() {
	addr := flag.String("addr", ":4000", "HTTP network address")
	dsn := flag.String("dsn", "web:pass@/snippetbox?parseTime=true", "MySQL data source name")

	flag.Parse()

	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	tlsConfig := &tls.Config{
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
	}

	db, err := openDb(*dsn)
	if err != nil {
		errLog.Fatal(err)
	}
	defer db.Close()
	templateCache, err := newTemplateCache()
	if err != nil {
		errLog.Fatal(err)
	}

	sessionManager := scs.New()
	sessionManager.Store = mysqlstore.New(db)
	sessionManager.Lifetime = 12 * time.Hour
	sessionManager.Cookie.Secure = true

	app := &application{
		errorLog:       errLog,
		infoLog:        infoLog,
		snippets:       &models.SnippetModel{Db: db},
		user:           &models.UserModel{DB: db},
		templateCache:  templateCache,
		formDecoder:    form.NewDecoder(),
		sessionManager: sessionManager,
	}

	srv := &http.Server{
		Addr:      *addr,
		ErrorLog:  errLog,
		Handler:   app.routes(),
		TLSConfig: tlsConfig,

		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	infoLog.Printf("Starting server on %s", *addr)
	err = srv.ListenAndServeTLS("./tls/cert.pem", "./tls/key.pem")
	errLog.Fatal(err)
}

func openDb(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func (a *application) routes() http.Handler {
	router := httprouter.New()

	router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.notFound(w)
	})

	fileServer := http.FileServer(http.Dir("./ui/static/"))
	router.Handler(http.MethodGet, "/static/*filepath", http.StripPrefix("/static", fileServer))

	dynamic := alice.New(a.sessionManager.LoadAndSave)

	router.Handler(http.MethodGet, "/", dynamic.ThenFunc(a.Home))
	router.Handler(http.MethodGet, "/snippet/view/:id", dynamic.ThenFunc(a.SnippetView))
	router.Handler(http.MethodGet, "/snippet/create", dynamic.ThenFunc(a.SnippetCreate))
	router.Handler(http.MethodPost, "/snippet/create", dynamic.ThenFunc(a.SnippetCreatePost))

	router.Handler(http.MethodGet, "/user/signup", dynamic.ThenFunc(a.userSignup))
	router.Handler(http.MethodPost, "/user/signup", dynamic.ThenFunc(a.userSignupPost))
	router.Handler(http.MethodGet, "/user/login", dynamic.ThenFunc(a.userLogin))
	router.Handler(http.MethodPost, "/user/login", dynamic.ThenFunc(a.userLoginPost))
	router.Handler(http.MethodPost, "/user/logout", dynamic.ThenFunc(a.userLogoutPost))

	standard := alice.New(a.recoverPanic, a.logRequest, secureHeaders)
	return standard.Then(router)
}
