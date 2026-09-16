# Tarea 06 — Directorio de estado explícito

## Objetivo
Permitir que un consumidor ubique exactamente el estado privado de DexGitHub sin cambiar el comportamiento legado de `RootDir`.

## Resultado
- `Config.StateDir` opcional.
- Si no se define, continúa usando `RootDir/.dexgithub`.
- `NewFileStoreAt(dir)` usa exactamente la ruta indicada.
- `NewFileStore(root)` mantiene compatibilidad histórica.
- Permisos 0700/0600, rechazo de symlinks y escritura atómica permanecen intactos.
- README y contexto técnico actualizados.

## Validación
- `gofmt`: OK.
- `go test ./...`: OK.
- `go test -race ./...`: OK.
- `go vet ./...`: OK.
- `git diff --check`: OK.
- Tests de ruta exacta y permisos: OK.

## Estado
Completada. Preparada para release local `v0.1.5`; no publicar hasta autorización explícita.
