// Package play runs the piano mode on native and browser platforms.
// Judgement and saved attempts live in internal/piano; mode-independent storyboard,
// UI, keyboard and clock code lives outside this scene. Play owns the audio bank
// (decoding, playback, pitch-preserving rate changes), the
// chat drawer state and score history used for multiplayer comparisons.
package play
