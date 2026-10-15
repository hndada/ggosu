import { tr } from './language.js';
export const defaultChat = () => ({ticker:false, autoHide:true, mentionPopup:true, messagePopup:true, mentionSound:false, instant:false});

export function validateChat(value) {
  const result = defaultChat();
  for (const key of Object.keys(result)) {
    if (value[key] === undefined) continue;
    if (typeof value[key] !== 'boolean') throw new Error(tr('채팅 설정 값을 확인해 주세요.'));
    result[key] = value[key];
  }
  return result;
}

// Match a whole nickname, including @name; mint should not match chocomint.
export function mentioned(text, name) {
  if (!name?.trim()) return false;
  const escaped = name.trim().replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return new RegExp(`(?<![\\p{L}\\p{N}_])${escaped}(?![\\p{L}\\p{N}_])`, 'iu').test(text);
}

export function alertFor(prefs, {text, name, own, visible, playing}) {
  if (own || visible) return {ticker:false,popup:false,sound:false,defer:false};
  const mention = mentioned(text, name);
  const popup = mention ? prefs.mentionPopup : prefs.messagePopup;
  const sound = mention && prefs.mentionSound;
  return {ticker:prefs.ticker, popup, sound, defer:playing && !prefs.instant && (popup || sound)};
}
