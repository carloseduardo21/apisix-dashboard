import type { Role } from '../api'

export const ROLE_LABEL: Record<Role, string> = { viewer: 'Leitura', operator: 'Operador', admin: 'Administrador' }
export const ROLE_COLOR: Record<Role, string> = { viewer: 'gray', operator: 'indigo', admin: 'grape' }

export function initials(name: string) {
  const parts = name.split(/[._\-\s]+/).filter(Boolean)
  return (parts.length > 1 ? parts[0][0] + parts[1][0] : name.slice(0, 2)).toUpperCase()
}

const AVATAR_COLORS = ['indigo', 'cyan', 'teal', 'grape', 'orange', 'pink', 'blue', 'violet']

/** Stable color per username. */
export function avatarColor(name: string) {
  let h = 0
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) >>> 0
  return AVATAR_COLORS[h % AVATAR_COLORS.length]
}
