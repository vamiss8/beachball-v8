// a copy of the server's player step, used to predict your own movement.
//
// this is the one place in the client that simulates anything, and it exists
// because waiting for the server before drawing your own player is what makes
// the controls feel soft: every key press costs a full round trip before
// anything moves.
//
// it stays honest in two ways. the numbers are not written down here at all —
// they arrive with the welcome message, so the server remains the only place
// they are decided. and the server still owns the outcome: every snapshot
// overwrites this state, and anything predicted wrongly is corrected. what
// follows must mirror player.go tick for tick, down to the order of the
// operations, or the two will disagree and the correction will be visible.

// clonePlayerState copies the parts of a snapshot player this file changes,
// so replaying never writes into the snapshot it started from
export function clonePlayerState(player) {
  return {
    pos: { x: player.pos.x, y: player.pos.y },
    velocityX: player.velocityX,
    velocityY: player.velocityY,
    rotation: player.rotation,
    isJumping: player.isJumping,
    isBlocking: player.isBlocking,
    motion: { ...player.motion },
  };
}

// stepPlayer advances one player by exactly one tick, in place
export function stepPlayer(state, input, side, arena, tuning) {
  stepHorizontal(state, input, side, arena, tuning);
  stepVertical(state, input, side, arena, tuning);
  stepRotation(state, side, tuning);
}

function stepHorizontal(state, input, side, arena, tuning) {
  const m = state.motion;

  if (m.dashCooldown > 0) m.dashCooldown--;

  dashOnDoubleTap(state, input, tuning);

  // two separate checks rather than an else, exactly as on the server: with
  // both keys down, right wins
  let walk = 0;
  if (input.left) walk = -tuning.moveSpeed;
  if (input.right) walk = tuning.moveSpeed;

  state.velocityX = walk + m.dashVelocity;
  state.pos.x += state.velocityX;
  m.dashVelocity *= tuning.dashFriction;

  clampToOwnHalf(state, side, arena, tuning);
}

function dashOnDoubleTap(state, input, tuning) {
  const m = state.motion;

  const pressedLeft = input.left && !m.prevLeft;
  const pressedRight = input.right && !m.prevRight;
  m.prevLeft = input.left;
  m.prevRight = input.right;

  // aged before this tick's presses, so a press lands on zero
  m.tapLeft = agePendingTap(m.tapLeft, tuning);
  m.tapRight = agePendingTap(m.tapRight, tuning);

  if (pressedLeft) {
    if (m.tapLeft >= 0) {
      startDash(state, -1, tuning);
      m.tapLeft = -1;
    } else {
      m.tapLeft = 0;
    }
  }

  if (pressedRight) {
    if (m.tapRight >= 0) {
      startDash(state, 1, tuning);
      m.tapRight = -1;
    } else {
      m.tapRight = 0;
    }
  }
}

function agePendingTap(age, tuning) {
  if (age < 0) return -1;
  age++;
  return age > tuning.doubleTapWindowTicks ? -1 : age;
}

function startDash(state, dir, tuning) {
  const m = state.motion;
  if (m.dashesLeft <= 0 || m.dashCooldown > 0) return;

  m.dashVelocity = dir * tuning.dashVelocity;
  m.rotationVel = dir * tuning.dashSpin;
  m.dashesLeft--;
  m.dashCooldown = tuning.dashCooldownTicks;
}

function clampToOwnHalf(state, side, arena, tuning) {
  let minX = 0;
  let maxX = arena.width - arena.playerWidth;

  if (side === 'left') {
    maxX = Math.min(maxX, arena.width / 2 - arena.playerWidth - tuning.netGap);
  } else {
    minX = Math.max(minX, arena.width / 2 + tuning.netGap);
  }

  if (state.pos.x < minX) state.pos.x = minX;
  if (state.pos.x > maxX) state.pos.x = maxX;
}

function stepVertical(state, input, side, arena, tuning) {
  const m = state.motion;

  // a jump fires on the rising edge only; holding the key does nothing
  const justPressed = input.jump && !m.prevJump;
  m.prevJump = input.jump;

  if (justPressed) {
    if (!state.isJumping) {
      state.velocityY = tuning.jumpVelocity;
      state.isJumping = true;
      m.canDoubleJump = true;
    } else if (m.canDoubleJump) {
      state.velocityY = tuning.doubleJumpVelocity;
      m.canDoubleJump = false;
      m.rotationVel = side === 'left' ? tuning.doubleJumpSpin : -tuning.doubleJumpSpin;
    }
  }

  state.isBlocking = input.block && state.isJumping;
  state.velocityY += state.isBlocking ? tuning.blockGravity : tuning.gravity;
  state.pos.y += state.velocityY;

  const ground = groundLevel(arena);
  if (state.pos.y >= ground) {
    state.pos.y = ground;
    state.velocityY = 0;
    state.rotation = 0;
    m.rotationVel = 0;
    state.isJumping = false;
    state.isBlocking = false;
    m.canDoubleJump = true;
    m.dashesLeft = tuning.dashesPerAirtime;
  }
}

function stepRotation(state, side, tuning) {
  const m = state.motion;

  if (state.isBlocking) {
    state.rotation = side === 'left' ? tuning.blockAngle : -tuning.blockAngle;
    m.rotationVel = 0;
    return;
  }

  if (m.rotationVel !== 0) {
    state.rotation += m.rotationVel;
    // one full turn, then settle upright
    if (Math.abs(state.rotation) >= 2 * Math.PI) {
      state.rotation = 0;
      m.rotationVel = 0;
    }
    return;
  }

  state.rotation = 0;
}

// groundLevel is the y where a player's top edge rests on the sand
function groundLevel(arena) {
  return arena.groundY - arena.playerHeight;
}
