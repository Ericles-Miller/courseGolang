package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"errors"
	"fmt"
	"io"
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

type Response struct {
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

func sendJSON(w http.ResponseWriter, resp Response, status int) {
	data, err := json.Marshal(resp)
	if err != nil {
		fmt.Println("error ao fazer marshal de json:", err)
		sendJSON(w, Response{Error: "something went wrong"}, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		fmt.Println("error ao enviar a resposta:", err)
		return
	}
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
		r.Post("/users", handlePostUsers(db))
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

		// Se o usuário não existir no map, retorna 404 com mensagem de erro em JSON.
		if !ok {
			sendJSON(w, Response{Error: "usuario nao encontrado"}, http.StatusNotFound)
			return
		}

		sendJSON(w, Response{Data: user}, http.StatusOK)
	}
}

// handlePostUsers retorna um handler que cria um novo usuário no db a partir do body da requisição.
// Usa closure para capturar o db, evitando variáveis globais.
func handlePostUsers(db map[int64]User) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Limita o tamanho do body a 1000 bytes para evitar payloads excessivamente grandes.
		r.Body = http.MaxBytesReader(w, r.Body, 1000)

		// Lê todo o conteúdo do body da requisição.
		data, err := io.ReadAll(r.Body)

		if err != nil {
			// errors.As verifica se o erro é do tipo MaxBytesError (body excedeu o limite).
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				sendJSON(w, Response{Error: "body too large"}, http.StatusRequestEntityTooLarge)
				return
			}

			// Para qualquer outro erro de leitura, loga no terminal e retorna 500.
			fmt.Println(err)
			sendJSON(w, Response{Error: "something went wrong"}, http.StatusInternalServerError)
			return
		}

		var user User

		// Deserializa o JSON do body para o struct User.
		// Retorna 422 se o JSON for inválido ou não corresponder aos campos esperados.
		if err := json.Unmarshal(data, &user); err != nil {
			sendJSON(w, Response{Error: "invalid body"}, http.StatusUnprocessableEntity)
			return
		}

		// Salva o usuário no map usando o ID como chave.
		db[user.ID] = user

		// Retorna 201 Created indicando que o recurso foi criado com sucesso.
		w.WriteHeader(http.StatusCreated)
	}
}
