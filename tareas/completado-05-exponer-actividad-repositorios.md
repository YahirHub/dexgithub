# Tarea 05 — Exponer actividad de repositorios

## Objetivo
Exponer en el modelo público de repositorios las marcas de tiempo de GitHub necesarias para ordenar por actividad real sin duplicar llamadas ni autenticación fuera de DexGitHub.

## Resultado
- `Repository` expone `CreatedAt`, `UpdatedAt` y `PushedAt`.
- Los endpoints de repositorios ya consumidos mapean `created_at`, `updated_at` y `pushed_at` sin requests adicionales.
- La API pública existente mantiene compatibilidad hacia atrás.
- README y contexto técnico actualizados.
- Sin dependencias nuevas.

## Validación
- `gofmt`: OK.
- `go test ./...`: OK.
- `go test -race ./...`: OK.
- `go vet ./...`: OK.
- `git diff --check`: OK.
- Regresión que comprueba el mapeo de `pushed_at`, `updated_at` y `created_at`: OK.

## Estado
Completada. Preparada para release local `v0.1.4`; no hacer push hasta autorización explícita.
