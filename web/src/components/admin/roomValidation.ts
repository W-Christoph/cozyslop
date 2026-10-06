// Mirror config.ValidRoomName and config.ValidateNekoURL; the server remains
// authoritative (including conflicts with configured rooms).
export function roomNameError(name: string): string {
  return /^[a-zA-Z0-9_-]+$/.test(name) ? ''
    : 'Room names must contain only letters, digits, underscores or hyphens.'
}

export function nekoUrlError(raw: string): string {
  const error = 'Neko URL must be an absolute http or https URL without credentials, query or fragment.'
  // Check the original text: URL normalizes missing slashes and whitespace,
  // and its search/hash omit an empty ?/#, which the server rejects.
  if (!/^https?:\/\/[^/?#]+/i.test(raw) || /[\u0000-\u001f\u007f?#]/.test(raw) || /%(?![\da-f]{2})/i.test(raw)) return error
  try {
    const authority = raw.slice(raw.indexOf('://') + 3).split('/')[0]
    if (/[\s\\@]/.test(authority)) return error
    if (!/^(?:\[[^\]]+\]|[^:[\]]+)(?::\d*)?$/.test(authority)) return error
    // Go accepts numeric ports without checking their range and IPv6 zone
    // identifiers. Validate the hostname without these browser restrictions.
    const host = authority.replace(/:\d*$/, '').replace(/%25[^\]]+(?=\]$)/i, '')
    const url = new URL(`http://${host}`)
    return url.hostname ? '' : error
  } catch {
    return error
  }
}
