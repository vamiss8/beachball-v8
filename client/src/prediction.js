// prediction of your own player: steps the same simulation locally so the
// controls answer immediately, then reconciles against every snapshot.
//
// the server stays authoritative. nothing here decides anything — each
// snapshot overwrites the predicted state, and this file only replays the
// inputs the server has not confirmed yet.

import { clonePlayerState, stepPlayer } from './predict.js';

// correction smaller than this is hidden by sliding the drawn position back
// over a few frames. anything larger is a real disagreement — a lost input,
// or a rejoin — and pretending otherwise would drag the player across the
// court instead of putting them where the server says they are
const SMOOTHING_LIMIT = 140;

// how much of the remaining visual offset survives each tick
const OFFSET_DECAY = 0.85;

export class Prediction {
  constructor(arena, tuning) {
    this.arena = arena;
    this.tuning = tuning;
    this.tickDuration = 1 / arena.tickRate;

    this.accumulator = 0;
    this.seq = 0;

    // inputs sent but not yet confirmed by a snapshot
    this.pending = [];

    this.state = null;
    this.side = null;
    this.offset = { x: 0, y: 0 };
  }

  // advance runs whole ticks worth of real time and returns the inputs that
  // now need sending. stepping at a fixed rate is what keeps this in step
  // with the server: a 144hz screen must not predict 144 ticks a second
  advance(dt, keys) {
    const produced = [];

    this.accumulator += dt;
    while (this.accumulator >= this.tickDuration) {
      this.accumulator -= this.tickDuration;
      this.seq++;

      // copied, since the live key state keeps changing under us
      const snapshot = { ...keys };
      const input = { seq: this.seq, keys: snapshot };

      this.pending.push(input);
      produced.push(input);

      if (this.state) {
        stepPlayer(this.state, snapshot, this.side, this.arena, this.tuning);
      }

      this.offset.x *= OFFSET_DECAY;
      this.offset.y *= OFFSET_DECAY;
    }

    return produced;
  }

  // reconcile rebuilds the prediction on top of what the server confirmed
  reconcile(player) {
    this.side = player.side;

    const previous = this.state && { x: this.state.pos.x, y: this.state.pos.y };

    // anything the server has already accounted for is history
    this.pending = this.pending.filter((input) => input.seq > player.lastInputSeq);

    const state = clonePlayerState(player);
    for (const input of this.pending) {
      stepPlayer(state, input.keys, player.side, this.arena, this.tuning);
    }
    this.state = state;

    if (previous) this.absorbCorrection(previous, state.pos);
  }

  // absorbCorrection turns a small disagreement into a visual offset that
  // decays, so the player never sees themselves jump a few pixels
  absorbCorrection(previous, corrected) {
    const dx = previous.x - corrected.x;
    const dy = previous.y - corrected.y;

    if (Math.hypot(dx, dy) > SMOOTHING_LIMIT) {
      this.offset.x = 0;
      this.offset.y = 0;
      return;
    }

    this.offset.x = dx;
    this.offset.y = dy;
  }

  // view is the predicted player as it should be drawn, offset and all
  view() {
    if (!this.state) return null;

    return {
      pos: { x: this.state.pos.x + this.offset.x, y: this.state.pos.y + this.offset.y },
      velocityX: this.state.velocityX,
      velocityY: this.state.velocityY,
      rotation: this.state.rotation,
      isJumping: this.state.isJumping,
      isBlocking: this.state.isBlocking,
    };
  }

  // forget drops everything, for a reconnect that starts a new player
  forget() {
    this.pending = [];
    this.state = null;
    this.seq = 0;
    this.offset = { x: 0, y: 0 };
  }
}
