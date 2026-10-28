package tooling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Def struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

const maxFetchRunes = 8000

var (
	reDropBlocks = regexp.MustCompile(`(?is)<(script|style|noscript|svg|head)\b.*?</(script|style|noscript|svg|head)>`)
	reTags       = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpaces     = regexp.MustCompile(`[ \t\r\f\v]+`)
	reNewlines   = regexp.MustCompile(`\n\s*\n+`)
)

// Defs lists built-in tools. web_search is only offered when Tavily is configured.
func Defs(tavily bool) []Def {
	defs := []Def{
		{"web_fetch", "Fetch a public web page by URL (e.g. a link from web_search). Returns cleaned text, truncated to about 8000 characters.", json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`)},
		{"get_datetime", "Get the current date and time. Use for any question about today's date, the time, or the day of week.", json.RawMessage(`{"type":"object","properties":{"timezone":{"type":"string","description":"IANA name, e.g. Asia/Jakarta. Default UTC."}}}`)},
		{"get_weather", "Get current weather and a 3-day forecast for a city.", json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`)},
		{"calculate", "Evaluate an arithmetic expression exactly. Supports + - * / % ^ parentheses, sqrt() and abs(). Use for any non-trivial math instead of computing mentally.", json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}},"required":["expression"]}`)},
	}
	if tavily {
		defs = append(defs, Def{"web_search", "Search the live web for current or realtime information (news, prices, recent events, anything after your training cutoff). Returns an answer summary plus titles, URLs and snippets.", json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"topic":{"type":"string","enum":["general","news"],"description":"Use news for recent events."},"days":{"type":"integer","minimum":1,"maximum":30,"description":"With topic=news, only results from the last N days."},"max_results":{"type":"integer","minimum":1,"maximum":10}},"required":["query"]}`)})
	}
	return defs
}

// RunBuiltin returns handled=false when name is not a built-in.
func RunBuiltin(ctx context.Context, name string, args json.RawMessage, tavilyKey string) (out string, handled bool, err error) {
	switch name {
	case "web_fetch":
		out, err = WebFetch(ctx, args)
	case "web_search":
		out, err = TavilySearch(ctx, tavilyKey, args)
	case "get_datetime":
		out, err = DateTime(args)
	case "get_weather":
		out, err = Weather(ctx, args)
	case "calculate":
		out, err = Calculate(args)
	default:
		return "", false, nil
	}
	return out, true, err
}

func IsBuiltin(name string) bool {
	switch name {
	case "web_fetch", "web_search", "get_datetime", "get_weather", "calculate", "remember":
		return true
	}
	return false
}

func cleanText(body string) string {
	if !strings.Contains(strings.ToLower(body[:min(len(body), 2048)]), "<") {
		return truncateRunes(body)
	}
	s := reDropBlocks.ReplaceAllString(body, " ")
	s = reTags.ReplaceAllString(s, "\n")
	s = htmlUnescape(s)
	s = reSpaces.ReplaceAllString(s, " ")
	s = reNewlines.ReplaceAllString(s, "\n")
	return truncateRunes(strings.TrimSpace(s))
}

func htmlUnescape(s string) string {
	return strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(s)
}

func truncateRunes(s string) string {
	r := []rune(s)
	if len(r) <= maxFetchRunes {
		return s
	}
	return string(r[:maxFetchRunes]) + "\n[truncated]"
}

func DateTime(args json.RawMessage) (string, error) {
	var in struct {
		Timezone string `json:"timezone"`
	}
	_ = json.Unmarshal(args, &in)
	loc := time.UTC
	if in.Timezone != "" {
		l, err := time.LoadLocation(in.Timezone)
		if err != nil {
			return "", fmt.Errorf("timezone tidak valid: %s", in.Timezone)
		}
		loc = l
	}
	now := time.Now().In(loc)
	return fmt.Sprintf("%s (%s), %s", now.Format("2006-01-02 15:04:05 MST"), loc, now.Weekday()), nil
}

func getJSON(ctx context.Context, rawURL string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("layanan mengembalikan status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(v)
}

func Weather(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		City string `json:"city"`
	}
	if err := json.Unmarshal(args, &in); err != nil || strings.TrimSpace(in.City) == "" {
		return "", errors.New("parameter city wajib diisi")
	}
	var geo struct {
		Results []struct {
			Name      string  `json:"name"`
			Country   string  `json:"country"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := getJSON(ctx, "https://geocoding-api.open-meteo.com/v1/search?count=1&name="+url.QueryEscape(in.City), &geo); err != nil {
		return "", err
	}
	if len(geo.Results) == 0 {
		return "", fmt.Errorf("kota %q tidak ditemukan", in.City)
	}
	g := geo.Results[0]
	var fc map[string]any
	q := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%v&longitude=%v&timezone=auto&forecast_days=3"+
		"&current=temperature_2m,apparent_temperature,relative_humidity_2m,precipitation,wind_speed_10m,weather_code"+
		"&daily=temperature_2m_max,temperature_2m_min,precipitation_sum,weather_code", g.Latitude, g.Longitude)
	if err := getJSON(ctx, q, &fc); err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]any{
		"place":    g.Name + ", " + g.Country,
		"forecast": fc,
		"note":     "weather_code is a WMO code: 0 clear, 1-3 cloudy, 45-48 fog, 51-67 rain/drizzle, 71-77 snow, 80-82 showers, 95-99 thunderstorm. Units: C, km/h, mm.",
	})
	return string(out), nil
}

func Calculate(args json.RawMessage) (string, error) {
	var in struct {
		Expression string `json:"expression"`
	}
	if err := json.Unmarshal(args, &in); err != nil || strings.TrimSpace(in.Expression) == "" {
		return "", errors.New("parameter expression wajib diisi")
	}
	expr, err := parser.ParseExpr(in.Expression)
	if err != nil {
		return "", errors.New("ekspresi tidak valid")
	}
	v, err := evalExpr(expr)
	if err != nil {
		return "", err
	}
	return strconv.FormatFloat(v, 'g', 15, 64), nil
}

func evalExpr(e ast.Expr) (float64, error) {
	switch n := e.(type) {
	case *ast.BasicLit:
		if n.Kind != token.INT && n.Kind != token.FLOAT {
			return 0, errors.New("literal tidak didukung")
		}
		return strconv.ParseFloat(n.Value, 64)
	case *ast.ParenExpr:
		return evalExpr(n.X)
	case *ast.UnaryExpr:
		x, err := evalExpr(n.X)
		if err != nil {
			return 0, err
		}
		switch n.Op {
		case token.SUB:
			return -x, nil
		case token.ADD:
			return x, nil
		}
	case *ast.BinaryExpr:
		x, err := evalExpr(n.X)
		if err != nil {
			return 0, err
		}
		y, err := evalExpr(n.Y)
		if err != nil {
			return 0, err
		}
		switch n.Op {
		case token.ADD:
			return x + y, nil
		case token.SUB:
			return x - y, nil
		case token.MUL:
			return x * y, nil
		case token.QUO, token.REM:
			if y == 0 {
				return 0, errors.New("pembagian dengan nol")
			}
			if n.Op == token.REM {
				return math.Mod(x, y), nil
			}
			return x / y, nil
		case token.XOR:
			return math.Pow(x, y), nil
		}
	case *ast.CallExpr:
		id, ok := n.Fun.(*ast.Ident)
		if ok && len(n.Args) == 1 && (id.Name == "sqrt" || id.Name == "abs") {
			x, err := evalExpr(n.Args[0])
			if err != nil {
				return 0, err
			}
			if id.Name == "abs" {
				return math.Abs(x), nil
			}
			if x < 0 {
				return 0, errors.New("sqrt negatif")
			}
			return math.Sqrt(x), nil
		}
	}
	return 0, errors.New("ekspresi tidak didukung")
}
