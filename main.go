package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	// chi.NewMux cria um novo roteador (multiplexer) do Chi.
	// É o ponto central onde todas as rotas e middlewares são registrados.
	r := chi.NewMux()

	// middleware.Recoverer captura panics em handlers e retorna HTTP 500,
	// evitando que o servidor caia por erros não tratados.
	r.Use(middleware.Recoverer)

	// middleware.RequestID injeta um ID único em cada requisição (via header X-Request-Id).
	// Útil para rastrear logs de uma mesma requisição.
	r.Use(middleware.RequestID)

	// middleware.Logger registra no terminal cada requisição recebida com método, rota e duração.
	r.Use(middleware.Logger)

	// Rota simples GET que retorna a data/hora atual do servidor.
	r.Get("/horario", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		fmt.Fprintln(w, now)
	})

	// r.Route agrupa rotas sob um prefixo comum ("/api").
	// Todas as sub-rotas definidas dentro herdam esse prefixo.
	r.Route("/api", func(r chi.Router) {

		// Versionamento de API: rotas sob "/api/v1"
		r.Route("/v1", func(r chi.Router) {
			// GET /api/v1/users — handler vazio, serve como exemplo de estrutura
			r.Get("/users", func(w http.ResponseWriter, r *http.Request) {})
		})

		// Versionamento de API: rotas sob "/api/v2" (ainda sem rotas definidas)
		r.Route("/v2", func(r chi.Router) {
		})

		// r.With aplica um middleware pontualmente, apenas para esta rota.
		// middleware.RealIP lê o IP real do cliente a partir de headers como X-Forwarded-For.
		// GET /api/users
		r.With(middleware.RealIP).Get("/users", func(w http.ResponseWriter, r *http.Request) {})

		// r.Group cria um sub-grupo de rotas sem alterar o prefixo da URL.
		// Permite aplicar middlewares apenas às rotas dentro do grupo.
		r.Group(func(r chi.Router) {
			// middleware.BasicAuth protege as rotas do grupo com autenticação HTTP Basic.
			// O mapa define as credenciais válidas: usuário "admin" com senha "admin".
			r.Use(middleware.BasicAuth("", map[string]string{
				"admin": "admin",
			}))

			// GET /api/healthcheck — retorna "ping" para indicar que o servidor está no ar.
			// Exige autenticação Basic (definida no grupo acima).
			r.Get("/healthcheck", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintln(w, "ping")
			})
		})
	})

	// Inicia o servidor HTTP na porta 8080.
	// Se houver erro ao subir (ex: porta ocupada), panic encerra o programa imediatamente.
	fmt.Println("Servidor rodando em http://localhost:8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		panic(err)
	}
}