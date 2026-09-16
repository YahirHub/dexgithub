# Fecha
2026-09-16

# Objetivo
Exponer las marcas de tiempo de actividad que GitHub ya devuelve en los repositorios autorizados para que los consumidores puedan ordenar por movimiento real sin hacer una consulta adicional por repositorio.

# Decisiones tomadas
- `Repository` incorpora `CreatedAt`, `UpdatedAt` y `PushedAt` como `time.Time` opcionales.
- `listRepositories` mapea `created_at`, `updated_at` y `pushed_at` de la respuesta de GitHub.
- No se agrega un método de ordenamiento dentro de DexGitHub: la librería expone los datos y cada consumidor decide el criterio visual.
- No se añaden requests adicionales, caché ni dependencias.

# Arquitectura actual
`AccessibleRepositories` y los métodos por instalación continúan usando los mismos endpoints y autenticación; el modelo devuelto conserva ahora los timestamps de actividad ya presentes en cada objeto repository.

# Librerías usadas
Sólo standard library (`time`).

# Archivos importantes modificados
- `models.go`
- `installations.go`
- `github_test.go`
- `README.md`

# Problemas encontrados
El endpoint de repositorios de una instalación no ofrece un parámetro de ordenamiento por actividad. Confiar en el orden de respuesta no garantiza obtener los repositorios con movimiento más reciente.

# Soluciones implementadas
DexGitHub conserva `pushed_at`, `updated_at` y `created_at` del proveedor. El panel Dex puede ordenar localmente por `PushedAt` y usar los otros timestamps como fallback sin multiplicar llamadas a la API.

# Pendientes
Ninguno dentro de la librería.

# Próximos pasos
Publicar `v0.1.4` cuando se autorice el push y actualizar consumidores que necesiten actividad real.
