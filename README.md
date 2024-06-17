# Sistema de Temperatura por CEP

## Pré-requisitos

-   Docker e Docker Compose instalados

## Como rodar o projeto

1. Clone o repositório
2. Navegue até o diretório do projeto
3. Execute `docker-compose up --build`

## Endpoints

-   Serviço A: `POST /consulta` com body `{"cep": "29902555"}`
-   Serviço B: `GET /clima?cep=29902555`

## Observabilidade

-   Zipkin: [http://localhost:9411](http://localhost:9411)
-   Prometheus: [http://localhost:9090](http://localhost:9090)
-   Grafana: [http://localhost:3000](http://localhost:3000) (admin/admin)
