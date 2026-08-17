/**
 * Copies text to the clipboard.
 *
 * navigator.clipboard only exists in a secure context, and a self-hosted paste
 * server is very often reached over plain http on a LAN address — so the
 * deprecated execCommand path is the one that actually runs there.
 */
export async function copyText(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // Denied or unavailable; fall through to the legacy path.
    }
  }

  const textarea = document.createElement("textarea")
  textarea.value = text
  textarea.setAttribute("readonly", "")
  textarea.style.position = "fixed"
  textarea.style.opacity = "0"
  document.body.appendChild(textarea)

  try {
    textarea.select()
    return document.execCommand("copy")
  } catch {
    return false
  } finally {
    document.body.removeChild(textarea)
  }
}
