# Projeto: APISIX Dashboard (Go + React)

## Arquitetura
- backend/: Go, API REST que conversa com a Admin API do APISIX
- frontend/: React + TypeScript (strict)

## Regras obrigatórias
- Nunca hardcodar segredos, URLs ou admin keys; tudo via env/config
- Toda chamada à Admin API passa pelo backend, nunca direto do browser
- Toda rota do backend exige autenticação; validar input em todo handler
- Erros nunca expõem stack trace ou detalhes internos ao cliente
- Não adicionar dependência nova sem justificar no PR

## Padrões
- Go: gofmt, golangci-lint sem warnings, erros com wrap (fmt.Errorf %w), testes table-driven
- React: ESLint + Prettier, sem `any`, componentes funcionais, sem dangerouslySetInnerHTML
- Commits: Conventional Commits (feat:, fix:, chore:...)

## Antes de considerar uma tarefa pronta
- `make lint test` passando
- `govulncheck ./...` e `npm audit --audit-level=high` sem achados novos
- Atualizar docs/CHANGELOG se mudou comportamento