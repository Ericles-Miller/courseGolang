// Package main é o ponto de entrada da aplicação: configura o logger estruturado
// (zap + slog) e sobe um servidor HTTP com roteamento via chi.
package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"os"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"context"
	"time"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
)

// User representa um usuário do sistema.
// A tag `json:",string"` faz o ID ser serializado como string no JSON (em vez de número).
// A tag `json:"-"` omite o campo Password na serialização — nunca expõe a senha.
type User struct {
	Username string
	ID       int64 `json:",string"`
	Role     string
	Password Password `json:"-"`
}

// Password é um tipo dedicado para senhas: impede que o valor apareça
// acidentalmente em logs ou na saída fmt.
type Password string

// String implementa fmt.Stringer — qualquer %v ou %s mostrará "[REDACTED]"
// em vez do valor real da senha.
func (p Password) String() string {
	return "[REDACTED]"
}

// LogValue implementa slog.LogValuer — garante que o slog também
// registre "[REDACTED]" ao logar um campo do tipo Password.
func (p Password) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}

// LevelFoo é um nível de log customizado abaixo de DEBUG (-4),
// usado para demonstrar como criar níveis arbitrários com slog.
const LevelFoo = slog.Level(-50)

// Response é o envelope padrão de todas as respostas JSON da API.
// Apenas um dos campos (Error ou Data) deve estar preenchido por resposta.
type Response struct {
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

// sendJSON serializa resp como JSON, define o status HTTP e escreve na resposta.
// Em caso de falha no marshal, registra o erro e retorna 500 automaticamente.
func sendJSON(w http.ResponseWriter, resp Response, status int) {
	data, err := json.Marshal(resp)
	if err != nil {
		slog.Error("error ao fazer marshal de json", "error", err)
		sendJSON(w, Response{Error: "something went wrong"}, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		slog.Error("error ao enviar a resposta", "error", err)
		return
	}
}

func main() {
	// zap.NewProduction cria um logger de alta performance com saida JSON
	// pre-configurado para producao (nivel Info, sem cores, timestamps UTC).
	z, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}

	// zapslog.NewHandler adapta o core do zap para a interface slog.Handler,
	// permitindo usar a API padrao slog com o backend de performance do zap.
	zs := slog.New(zapslog.NewHandler(z.Core(), nil))
	zs.Info("Uma mensagem de teste")

	// Demonstracao do tipo Password: o valor nunca aparece em logs.
	p := Password("123456")
	u := User{Password: p}
	slog.Info("password", "u", u)

	// HandlerOptions configura o comportamento do handler JSON:
	// - AddSource: inclui arquivo e linha de onde o log foi chamado.
	// - Level: define o nivel minimo; LevelFoo (-50) aceita qualquer log.
	// - ReplaceAttr: reescreve atributos antes de serializar — aqui troca
	//   o label "DEBUG-46" pelo alias legivel "FOO".
	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     LevelFoo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "level" {
				level := a.Value.String()
				if level == "DEBUG-46" {
					a.Value = slog.StringValue("FOO")
				}
			}
			return a
		},
	}

	// NewJSONHandler escreve cada log como uma linha JSON no stdout.
	l := slog.New(slog.NewJSONHandler(os.Stdout, opts))

	// SetDefault substitui o logger global do pacote slog por l,
	// fazendo slog.Info/Debug/etc. usarem este handler a partir daqui.
	slog.SetDefault(l)
	slog.Debug("foo")
	slog.Info("Servico sendo iniciado", "version", "1.0.0")

	// l.With adiciona campos fixos a todas as mensagens do logger derivado.
	// slog.Group agrupa os campos sob uma chave comum no JSON ("app_info").
	l = l.With(slog.Group("app_info", slog.String("version", "1.0.0.")))
	l.Info("this is a test", "user", u)

	// LogAttrs e mais eficiente que Info/Debug pois evita alocacoes extras
	// ao aceitar slog.Attr diretamente em vez de pares chave/valor interface{}.
	l.LogAttrs(context.Background(), LevelFoo, "qualquer mensagem")
	l.LogAttrs(
		context.Background(),
		slog.LevelInfo,
		"tivemos um http request",
		// slog.Group agrupa campos relacionados sob uma chave no JSON,
		// facilitando filtragem e leitura em ferramentas como Datadog/Loki.
		slog.Group("http_data",
			slog.String("method", http.MethodDelete),
			slog.Int("status", http.StatusOK),
		),
		slog.Duration("time_taken", time.Second),
		slog.String("user_agent", "ahsiduas"),
	)

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
			// errors.AsType verifica se o erro é do tipo MaxBytesError (body excedeu o limite).
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				sendJSON(w, Response{Error: "body too large"}, http.StatusRequestEntityTooLarge)
				return
			}

			// Para qualquer outro erro de leitura, loga no terminal e retorna 500.
			slog.Error("falha ao ler o json do usuario", "error", err)
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
