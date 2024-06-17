package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/openzipkin/zipkin-go"
	zipkinhttp "github.com/openzipkin/zipkin-go/middleware/http"
	reporterhttp "github.com/openzipkin/zipkin-go/reporter/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type RequestBody struct {
	Cep string `json:"cep"`
}

type WeatherResponse struct {
	City  string  `json:"city"`
	TempC float64 `json:"temp_C"`
	TempF float64 `json:"temp_F"`
	TempK float64 `json:"temp_K"`
}

var tracer *zipkin.Tracer

func initTracer() {
	reporter := reporterhttp.NewReporter("http://zipkin:9411/api/v2/spans")
	endpoint, err := zipkin.NewEndpoint("servico-a", "localhost:8080")
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
	http.HandleFunc("/consulta", handleQuery)
	port := ":8080"
	fmt.Printf("Server listening on port %s\n", port)
	if err := http.ListenAndServe(port, zipkinhttp.NewServerMiddleware(tracer)(http.DefaultServeMux)); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	span := tracer.StartSpan("handleQuery")
	defer span.Finish()

	var requestBody RequestBody
	err := json.NewDecoder(r.Body).Decode(&requestBody)
	if err != nil {
		http.Error(w, "Failed to decode request body", http.StatusBadRequest)
		return
	}

	if requestBody.Cep == "" {
		http.Error(w, "CEP parameter is required", http.StatusBadRequest)
		return
	}

	if len(requestBody.Cep) != 8 {
		http.Error(w, "invalid zipcode", http.StatusUnprocessableEntity)
		return
	}

	url := fmt.Sprintf("http://servico-b:8081/clima?cep=%s", requestBody.Cep)

	client, err := zipkinhttp.NewClient(tracer, zipkinhttp.ClientTrace(true))
	if err != nil {
		http.Error(w, "Failed to create HTTP client", http.StatusInternalServerError)
		return
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}
	req = req.WithContext(r.Context())

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Failed to communicate with Service B", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorMsg struct {
			Error string `json:"error"`
		}
		err = json.NewDecoder(resp.Body).Decode(&errorMsg)
		if err != nil {
			http.Error(w, "Service B failed to process the request", http.StatusInternalServerError)
			return
		}

		http.Error(w, errorMsg.Error, resp.StatusCode)
		return
	}

	var weatherResponse WeatherResponse
	err = json.NewDecoder(resp.Body).Decode(&weatherResponse)
	if err != nil {
		http.Error(w, "Failed to decode response from Service B", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(weatherResponse)
}
