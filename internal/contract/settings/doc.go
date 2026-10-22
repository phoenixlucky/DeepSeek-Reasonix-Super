// Package settings is the one declaration of every user-file key: who may
// change it (Class), when a change reaches the host (Effect) and whether its
// value is ever shown (Secret). The model, the cards and the screens read it
// instead of each keeping a list of their own.
//
// Class has no zero value and the declarations are held to the Config type in
// both directions, so a key added to the config cannot reach a frontend
// without someone deciding whether the model may touch it. A Guarded key is
// refused by Write itself, so the gate that keeps the model out of its own
// boundary has one place to live. Consent, the click, is the controller's.
package settings
