# Fecha
2026-09-16

# Objetivo
Permitir que cada consumidor de DexGitHub ubique exactamente el estado privado de la integración sin romper el comportamiento histórico basado en `RootDir/.dexgithub`.

# Decisiones tomadas
- `Config.StateDir` es opcional.
- Si `StateDir` está vacío, se normaliza a `RootDir/.dexgithub`, por lo que los consumidores existentes no cambian de ruta.
- `NewFileStore(root)` conserva su contrato histórico y sigue almacenando en `<root>/.dexgithub`.
- Se añade `NewFileStoreAt(dir)` para utilizar exactamente el directorio indicado.
- Las protecciones existentes permanecen: directorio `0700`, archivos `0600`, rechazo de symlinks y escrituras atómicas.
- La librería no migra rutas de productos. La migración pertenece al consumidor porque sólo él conoce el layout anterior y nuevo.

# Arquitectura actual
`Config.normalized()` resuelve `RootDir`, `StateDir` y `CloneRoot` por separado. Cuando no se inyecta un `Store`, DexGitHub crea un `FileStore` exactamente sobre `StateDir`.

# Archivos modificados
- `config.go`
- `storage.go`
- `storage_test.go`
- `README.md`

# Pruebas
- compatibilidad de `NewFileStore(root)`;
- almacenamiento exacto mediante `NewFileStoreAt`;
- `Config.StateDir` se conserva como ruta exacta;
- permisos POSIX del directorio privado;
- suite normal, race y vet.

# Pendientes
Ninguno dentro de la librería.
