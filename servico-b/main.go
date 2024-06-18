package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"

	"github.com/openzipkin/zipkin-go"
	zipkinhttp "github.com/openzipkin/zipkin-go/middleware/http"
	reporterhttp "github.com/openzipkin/zipkin-go/reporter/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type ViaCEPResponse struct {
	Localidade string `json:"localidade"`
	Erro       bool   `json:"erro"`
}

type WeatherAPIResponse struct {
	Current struct {
		TempC float64 `json:"temp_c"`
		TempF float64 `json:"temp_f"`
	} `json:"current"`
}

type WeatherResponse struct {
	TempC float64 `json:"temp_C"`
	TempF float64 `json:"temp_F"`
	TempK float64 `json:"temp_K"`
	City  string  `json:"city"`
}

type HttpClient interface {
	Get(url string) (*http.Response, error)
}

var httpClient HttpClient = &http.Client{}
var tracer *zipkin.Tracer

func initTracer() {
	reporter := reporterhttp.NewReporter("http://zipkin:9411/api/v2/spans")
	endpoint, err := zipkin.NewEndpoint("servico-b", "localhost:8081")
	if err != nil {
		log.Fatalf("failed to create endpoint: %v", err)
	}
	tracer, err = zipkin.NewTracer(reporter, zipkin.WithLocalEndpoint(endpoint))
	if err != nil {
		log.Fatalf("failed to create tracer: %v", err)
	}
}

func main() {
	initTracer()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/clima", handleWeatherRequest)
	port := ":8081"
	fmt.Printf("Server listening on port %s\n", port)
	if err := http.ListenAndServe(port, zipkinhttp.NewServerMiddleware(tracer)(http.DefaultServeMux)); err != nil {
		log.Panic("Server failed to start:", err)
	}
}

func handleWeatherRequest(w http.ResponseWriter, r *http.Request) {
	span, ctx := tracer.StartSpanFromContext(r.Context(), "handleWeatherRequest")
	defer span.Finish()

	cep := r.URL.Query().Get("cep")
	if len(cep) != 8 {
		http.Error(w, "invalid zipcode", http.StatusUnprocessableEntity)
		return
	}

	location, err := getLocationByCEP(ctx, cep)
	if err != nil {
		errorResponse := struct {
			Error      string `json:"error"`
			StatusCode int    `json:"statuscode"`
		}{
			Error:      "zip code not found",
			StatusCode: http.StatusNotFound,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(errorResponse)
		return
	}

	weather, err := getWeatherByLocation(ctx, location)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := WeatherResponse{
		TempC: weather.Current.TempC,
		TempF: weather.Current.TempF,
		TempK: celsiusToKelvin(weather.Current.TempC),
		City:  location,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func getLocationByCEP(ctx context.Context, cep string) (string, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "getLocationByCEP")
	defer span.Finish()

	url := fmt.Sprintf("https://viacep.com.br/ws/%s/json/", cep)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch data from ViaCEP: %v", err)
	}
	defer resp.Body.Close()

	var viaCEP ViaCEPResponse
	err = json.NewDecoder(resp.Body).Decode(&viaCEP)
	if err != nil {
		return "", fmt.Errorf("failed to decode ViaCEP response: %v", err)
	}

	if viaCEP.Erro {
		return "", fmt.Errorf("zipcode not found")
	}

	return viaCEP.Localidade, nil
}

func getWeatherByLocation(ctx context.Context, location string) (*WeatherAPIResponse, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "getWeatherByLocation")
	defer span.Finish()

	apiKey := "0d955ca900874ca3a08200551241606"

	sanitizedLocation := url.QueryEscape(location)
	url := fmt.Sprintf("http://api.weatherapi.com/v1/current.json?key=%s&q=%s", apiKey, sanitizedLocation)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch data from WeatherAPI: %v", err)
	}
	defer resp.Body.Close()

	var weather WeatherAPIResponse
	err = json.NewDecoder(resp.Body).Decode(&weather)
	if err != nil {
		return nil, fmt.Errorf("failed to decode WeatherAPI response: %v", err)
	}

	return &weather, nil
}

func celsiusToKelvin(celsius float64) float64 {
	return roundToPrecision(celsius+273.15, 1)
}

func roundToPrecision(value float64, precision int) float64 {
	p := math.Pow(10, float64(precision))
	return math.Round(value*p) / p
}
