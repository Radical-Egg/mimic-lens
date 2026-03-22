package main

import (
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/radical-egg/mimic-lens/internal/a2s"
)

type PageData struct {
	Host      string
	QueryPort string
	GamePort  string

	Submitted bool
	Error     string

	QueryResult *a2s.QueryResult
	GameProbe   *a2s.ProbeResult
}

type App struct {
	templates *template.Template
}

func main() {
	tmpl := template.Must(template.ParseFiles("templates/index.html"))

	app := &App{
		templates: tmpl,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.indexHandler())
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	log.Println("mimic-lens listening on :8080")
	log.Fatal(server.ListenAndServe())
}

func (a *App) indexHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			a.render(w, "index", PageData{})
			return

		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				a.render(w, "index", PageData{
					Submitted: true,
					Error:     "Failed to parse form submission",
				})
				return
			}

			host := strings.TrimSpace(r.FormValue("host"))
			queryPortStr := strings.TrimSpace(r.FormValue("query_port"))
			gamePortStr := strings.TrimSpace(r.FormValue("game_port"))

			data := PageData{
				Host:      host,
				QueryPort: queryPortStr,
				GamePort:  gamePortStr,
				Submitted: true,
			}

			queryPort, gamePort, err := validateInput(host, queryPortStr, gamePortStr)
			if err != nil {
				data.Error = err.Error()
				a.render(w, "index", data)
				return
			}

			queryResult, queryErr := a2s.QueryInfo(host, queryPort)
			gameProbe, gameErr := a2s.ProbeUDP(host, gamePort)

			data.QueryResult = queryResult
			data.GameProbe = gameProbe

			if queryErr != nil || gameErr != nil {
				data.Error = buildCombinedError(queryErr, gameErr)
			}

			a.render(w, "index", data)
			return

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}
}

func (a *App) render(w http.ResponseWriter, name string, data PageData) {
	if err := a.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template render error: %v", err)
		http.Error(w, "template rendering error", http.StatusInternalServerError)
	}
}

func validateInput(host, queryPortStr, gamePortStr string) (int, int, error) {
	if host == "" {
		return 0, 0, errors.New("Host is required")
	}

	if queryPortStr == "" {
		return 0, 0, errors.New("Query port is required")
	}

	if gamePortStr == "" {
		return 0, 0, errors.New("Game port is required")
	}

	queryPort, err := strconv.Atoi(queryPortStr)
	if err != nil {
		return 0, 0, errors.New("Query port must be a valid number")
	}
	if queryPort < 1 || queryPort > 65535 {
		return 0, 0, errors.New("Query port must be between 1 and 65535")
	}

	gamePort, err := strconv.Atoi(gamePortStr)
	if err != nil {
		return 0, 0, errors.New("Game port must be a valid number")
	}
	if gamePort < 1 || gamePort > 65535 {
		return 0, 0, errors.New("Game port must be between 1 and 65535")
	}

	return queryPort, gamePort, nil
}

func buildCombinedError(queryErr, gameErr error) string {
	switch {
	case queryErr != nil && gameErr != nil:
		return "Query port check: " + queryErr.Error() + " | Game port probe: " + gameErr.Error()
	case queryErr != nil:
		return "Query port check: " + queryErr.Error()
	case gameErr != nil:
		return "Game port probe: " + gameErr.Error()
	default:
		return ""
	}
}
