package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// User representa um usuário do sistema.
// A tag `json:",string"` faz o ID ser serializado como string no JSON (em vez de número).
// A tag `json:"-"` omite o campo Password na serialização — nunca expõe a senha.
type User struct {
	Username string
	ID       int64 `json:",string"`
	Role     string
	Password string `json:"-"`
}

func main() {
	// chi.NewMux cria o roteador principal onde rotas e middlewares são registrados.
	r := chi.NewMux()

	// middleware.Recoverer captura panics nos handlers e retorna HTTP 500,
	// evitando que o servidor caia por erros não tratados.
	r.Use(middleware.Recoverer)

	// middleware.RequestID injeta um ID único em cada requisição (header X-Request-Id),
	// útil para rastrear logs de uma mesma requisição.
	r.Use(middleware.RequestID)

	// middleware.Logger registra no terminal cada requisição com método, rota e duração.
	r.Use(middleware.Logger)

	// db simula um banco de dados em memória: um map onde a chave é o ID do usuário.
	db := map[int64]User{
		1: {
			Username: "admin",
			Password: "admin",
			Role:     "admin",
			ID:       1,
		},
	}

	// r.Group agrupa rotas sem alterar o prefixo da URL.
	// Permite aplicar middlewares apenas às rotas dentro do grupo.
	r.Group(func(r chi.Router) {
		// jsonMiddleware define Content-Type: application/json para todas as rotas do grupo.
		r.Use(jsonMiddleware)

		// GET /users/{id} — busca um usuário pelo ID.
		// O padrão [0-9]+ garante que apenas números são aceitos como parâmetro.
		r.Get("/users/{id:[0-9]+}", handleGetUsers(db))

		// POST /users — cria um novo usuário.
		r.Post("/users", handlePostUsers)
	})

	// Inicia o servidor HTTP na porta 8080.
	// Se houver erro ao subir (ex: porta ocupada), panic encerra o programa imediatamente.
	if err := http.ListenAndServe(":8080", r); err != nil {
		panic(err)
	}
}

// jsonMiddleware é um middleware que define o header Content-Type como application/json
// em todas as respostas, antes de chamar o próximo handler da cadeia.
func jsonMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// handleGetUsers retorna um handler que busca um usuário no db pelo ID da URL.
// Usa closure para capturar o db, evitando variáveis globais.
func handleGetUsers(db map[int64]User) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extrai o parâmetro "id" da URL e converte para int64.
		idStr := chi.URLParam(r, "id")
		id, _ := strconv.ParseInt(idStr, 10, 64)

		// Busca o usuário no map; ok será false se o ID não existir.
		user, ok := db[id]

		if ok {
			// json.Marshal serializa o struct User para JSON,
			// respeitando as tags (omite Password, converte ID para string).
			data, err := json.Marshal(user)
			if err != nil {
				panic(err)
			}

			_, _ = w.Write(data)
		}
	}
}

// handlePostUsers é o handler para criação de usuários — ainda sem implementação.
func handlePostUsers(w http.ResponseWriter, r *http.Request) {

}
