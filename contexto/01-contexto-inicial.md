# Fecha
2026-09-15

# Objetivo
Crear una librería Go standalone para integrar GitHub App y operaciones Git reutilizables por Dex y otros servicios.

# Decisiones tomadas
- Módulo previsto: `github.com/YahirHub/dexgithub`.
- Default de clones: `/root/.github/repos`.
- GitHub App como mecanismo principal; no PAT persistentes.
- Flujo Manifest para crear la GitHub App de forma guiada desde un panel.
- Tokens de instalación de corta duración para repos privados.
- Nunca persistir tokens en remote URLs, argumentos Git ni archivos `.git/config`.
- Usar el ejecutable Git nativo para máxima compatibilidad con repos reales, submódulos, LFS/configuración futura y evitar una dependencia Go Git pesada.
- Operaciones Git mediante `exec.CommandContext` con argumentos directos, límites y timeouts.
- Persistencia local root-only para credenciales/configuración; interfaz de almacenamiento preparada para poder sustituirla en Dex si luego se necesita un secret store distinto.
- No integrar todavía UI en Dex; primero terminar y verificar la librería standalone.

# Arquitectura actual
Proyecto nuevo. Se dividirá por responsabilidades reales: cliente GitHub/API, autenticación GitHub App, almacenamiento local y gestor Git.

# Librerías usadas
Inicialmente Go standard library y el ejecutable `git` del host. Se evitarán dependencias externas salvo necesidad demostrada.

# Archivos importantes modificados
- `.gitignore`
- `contexto/01-contexto-inicial.md`
- `tareas/en-proceso-01-integracion-github-app-y-git.md`

# Problemas encontrados
Ninguno todavía.

# Soluciones implementadas
Estructura inicial y decisiones de seguridad.

# Pendientes
Implementación completa, pruebas, documentación y commit.

# Próximos pasos
Crear `go.mod`, API pública, persistencia segura, cliente GitHub App y operaciones Git.
