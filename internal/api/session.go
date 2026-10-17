package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/plugin"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/utils"
	"github.com/stashapp/stash/ui"
)

const (
	returnURLParam = "returnURL"
	// switchParam opens the login page in switch mode
	switchParam = "switch"

	defaultLocale = "en-GB"
)

func getLoginPage() []byte {
	data, err := fs.ReadFile(ui.LoginUIBox, "login.html")
	if err != nil {
		panic(err)
	}
	return data
}

type loginTemplateData struct {
	URL   string
	Error string
	// AutoLoginUser is the user that the request would be logged in as
	// automatically, offered as a way to continue or to switch to
	AutoLoginUser string
	ContinueURL   string
	SwitchMode    bool
	CurrentUser   string

	ThemeColor        string
	AppStylesheets    []template.HTML
	PluginStylesheets []string
}

var (
	linkTagRE       = regexp.MustCompile(`<link\b[^>]*>`)
	relStylesheetRE = regexp.MustCompile(`\brel="stylesheet"`)
	hrefRE          = regexp.MustCompile(`\bhref="([^"]*)"`)
)

// appStylesheets returns the stylesheet link tags under /assets of the index
// page of the UI, which link the CSS bundle of the app by its hashed name.
func appStylesheets(uiFS fs.FS) []template.HTML {
	data, err := fs.ReadFile(uiFS, "index.html")
	if err != nil {
		logger.Debugf("error reading the index page of the UI: %v", err)
		return nil
	}

	var ret []template.HTML
	for _, tag := range linkTagRE.FindAllString(string(data), -1) {
		href := hrefRE.FindStringSubmatch(tag)
		if !relStylesheetRE.MatchString(tag) || href == nil {
			continue
		}

		if p := strings.TrimLeft(strings.TrimPrefix(href[1], "."), "/"); strings.HasPrefix(p, "assets/") {
			// the index page is served as is too
			ret = append(ret, template.HTML(tag))
		}
	}

	return ret
}

// sortPlugins orders the plugins as the UI does (sortPlugins in
// ui/v2.5/src/plugins.tsx): each plugin after the plugins it requires.
func sortPlugins(plugins []*plugin.Plugin) []*plugin.Plugin {
	var ids []string
	afters := make(map[string][]string)
	addNode := func(id string) {
		if _, found := afters[id]; !found {
			afters[id] = nil
			ids = append(ids, id)
		}
	}

	for _, p := range plugins {
		addNode(p.ID)
		for _, required := range p.UI.Requires {
			addNode(required)
			if !slices.Contains(afters[required], p.ID) {
				afters[required] = append(afters[required], p.ID)
			}
		}
	}

	var sorted []*plugin.Plugin
	visited := make(map[string]bool)
	var visit func(id string)
	visit = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true

		for _, after := range afters[id] {
			visit(after)
		}

		if i := slices.IndexFunc(plugins, func(p *plugin.Plugin) bool { return p.ID == id }); i >= 0 {
			sorted = slices.Insert(sorted, 0, plugins[i])
		}
	}

	for _, id := range ids {
		visit(id)
	}

	return sorted
}

// pluginStylesheets returns the CSS URLs of the enabled UI plugins, in the
// order that the UI loads them (see ui/v2.5/src/plugins.tsx).
func pluginStylesheets(ctx context.Context, plugins []*plugin.Plugin) []string {
	baseURL, _ := ctx.Value(BaseURLCtxKey).(string)

	var ret []string
	for _, p := range sortPlugins(plugins) {
		if !p.Enabled {
			continue
		}

		for _, u := range (pluginURLBuilder{BaseURL: baseURL, Plugin: p}).css() {
			if !slices.Contains(ret, u) {
				ret = append(ret, u)
			}
		}
	}

	return ret
}

// sameOriginURL returns u if it is a path on this server, and "" otherwise,
// so that the login page never redirects to another site.
func sameOriginURL(u string) string {
	// browsers read a backslash as a slash, and drop tabs and newlines
	if u == "" || strings.ContainsAny(u, `\`+"\t\r\n") {
		return ""
	}

	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil {
		return ""
	}

	// a path, but not a protocol-relative URL
	if !strings.HasPrefix(u, "/") || strings.HasPrefix(u, "//") {
		return ""
	}

	return u
}

func serveLoginPage(w http.ResponseWriter, r *http.Request, data loginTemplateData, plugins []*plugin.Plugin) {
	loginPage := string(getLoginPage())
	prefix := getProxyPrefix(r)
	loginPage = strings.ReplaceAll(loginPage, "/%BASE_URL%", prefix)

	data.ContinueURL = data.URL
	if data.ContinueURL == "" {
		data.ContinueURL = prefix + "/"
	}

	templ, err := template.New("Login").Parse(loginPage)
	if err != nil {
		http.Error(w, fmt.Sprintf("error: %s", err), http.StatusInternalServerError)
		return
	}

	buffer := bytes.Buffer{}
	err = templ.Execute(&buffer, data)
	if err != nil {
		http.Error(w, fmt.Sprintf("error: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")

	// the page loads the CSS of the plugins
	setPageSecurityHeaders(w, r, plugins)

	utils.ServeStaticContent(w, r, buffer.Bytes())
}

func handleLoginLocale(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// get the locale from the config
		lang := cfg.GetLanguage()
		if lang == "" {
			lang = defaultLocale
		}

		data, err := getLoginLocale(lang)
		if err != nil {
			logger.Debugf("Failed to load login locale file for language %s: %v", lang, err)
			// try again with the default language
			if lang != defaultLocale {
				data, err = getLoginLocale(defaultLocale)
				if err != nil {
					logger.Errorf("Failed to load login locale file for default language %s: %v", defaultLocale, err)
				}
			}

			// if there's still an error, response with an internal server error
			if err != nil {
				http.Error(w, "Failed to load login locale file", http.StatusInternalServerError)
				return
			}
		}

		// write a script to set the locale string map as a global variable
		localeScript := fmt.Sprintf("var localeStrings = %s;", data)
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(localeScript))
	}
}

func getLoginLocale(lang string) ([]byte, error) {
	data, err := fs.ReadFile(ui.LoginUIBox, "locales/"+lang+".json")
	if err != nil {
		return nil, err
	}
	return data, nil
}

func handleLogin(s manager.UserService, cfg *config.Config, uiFS fs.FS, pluginCache *plugin.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		returnURL := sameOriginURL(r.URL.Query().Get(returnURLParam))

		if !s.LoginRequired(ctx) {
			if returnURL != "" {
				http.Redirect(w, r, returnURL, http.StatusFound)
			} else {
				prefix := getProxyPrefix(r)
				http.Redirect(w, r, prefix+"/", http.StatusFound)
			}
			return
		}

		data := loginTemplateData{URL: returnURL}

		// switch mode changes the account of a signed in user
		current := models.GetCurrentUser(ctx)
		if current != nil && r.URL.Query().Get(switchParam) != "" {
			data.SwitchMode = true
			data.CurrentUser = current.Username
		}

		// the login page is never logged in automatically, so that another
		// user can log in. Without a session, it links to the return URL
		// instead, which is logged in automatically. In switch mode, it
		// offers to switch to the auto login user.
		if current == nil || data.SwitchMode {
			if u, _, err := autoLoginUser(ctx, s); err == nil && u != nil && (current == nil || u.Username != current.Username) {
				data.AutoLoginUser = u.Username
			}
		}

		// the stylesheets of the app; the page links the custom CSS itself
		plugins := pluginCache.ListPlugins()
		data.ThemeColor = cfg.GetThemeColor()
		data.AppStylesheets = appStylesheets(uiFS)
		if !cfg.GetDisableCustomizations() {
			data.PluginStylesheets = pluginStylesheets(ctx, plugins)
		}

		serveLoginPage(w, r, data, plugins)
	}
}

// handleLoginAuto replaces the session with one of the user marked for auto
// login. It applies the same checks as automatic login, and is refused
// otherwise.
func handleLoginAuto(g UserAuthenticator, getStore func() *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !g.LoginRequired(r.Context()) {
			http.Error(w, "Automatic login is not available", http.StatusForbidden)
			return
		}

		u, err := autoLogin(w, r, g, getStore())
		if err != nil {
			logger.Errorf("error switching to the auto login user: %v", err)
			http.Error(w, "An unexpected error occurred. See logs", http.StatusInternalServerError)
			return
		}

		if u == nil {
			http.Error(w, "Automatic login is not available", http.StatusForbidden)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func handleLoginPost(getStore func() *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := getStore().Login(w, r)
		if err != nil {
			// always log the error
			logger.Errorf("Error logging in: %v from IP: %s", err, r.RemoteAddr)
		}

		var invalidCredentialsError *session.InvalidCredentialsError

		if errors.As(err, &invalidCredentialsError) {
			http.Error(w, "Username or password is invalid", http.StatusUnauthorized)
			return
		}

		if err != nil {
			// don't expose the error to the user
			http.Error(w, "An unexpected error occurred. See logs", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func handleLogout(s manager.UserService, getStore func() *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := getStore().Logout(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// redirect to the login page if credentials are required
		prefix := getProxyPrefix(r)
		if s.LoginRequired(r.Context()) {
			http.Redirect(w, r, prefix+loginEndpoint, http.StatusFound)
		} else {
			http.Redirect(w, r, prefix+"/", http.StatusFound)
		}
	}
}
