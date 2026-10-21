// Package sitting owns every open Sitting of a Session: one uninterrupted
// stretch of reading a Source, from opening it to closing it. See CONTEXT.md.
//
// The Tracker keeps the rules a Sitting's end must follow in one place: the
// order of its writes (notes, progress marker, reading time, reschedule), that
// a peek records nothing, and that ending twice does nothing. Callers report
// what happened; they never re-state the policy.
package sitting
