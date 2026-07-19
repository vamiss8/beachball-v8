import * as p2 from 'p2-es';

// init canvas
const canvas = document.createElement('canvas');
const ctx = canvas.getContext('2d');
canvas.width = 800;
canvas.height = 600;
document.body.appendChild(canvas);

// init physics world with gravity
const world = new p2.World({
  gravity: [0, -9.81] 
});

// materials for bounce logic
const groundMaterial = new p2.Material();
const ballMaterial = new p2.Material();
const playerMaterial = new p2.Material();

// contact logic (restitution = bounciness)
world.addContactMaterial(new p2.ContactMaterial(groundMaterial, ballMaterial, {
  restitution: 0.8
}));
world.addContactMaterial(new p2.ContactMaterial(playerMaterial, ballMaterial, {
  restitution: 0.5 
}));

// setup static ground
const groundBody = new p2.Body({
  mass: 0,
  position: [0, -2]
});
const groundShape = new p2.Plane();
groundShape.material = groundMaterial;
groundBody.addShape(groundShape);
world.addBody(groundBody);

// create the hovering ball
const ballBody = new p2.Body({
  mass: 0, 
  position: [0, 5] 
});
const ballShape = new p2.Circle({ radius: 0.5 });
ballShape.material = ballMaterial;
ballBody.addShape(ballShape);
world.addBody(ballBody);

// player setup
const playerBody = new p2.Body({
  mass: 5, // player is heavier than the ball
  position: [-3, -1],
  fixedRotation: true // prevent player from rolling like a ball
});
const playerShape = new p2.Circle({ radius: 0.8 });
playerShape.material = playerMaterial;
playerBody.addShape(playerShape);
world.addBody(playerBody);

// input state
const keys = { right: false, left: false, up: false };

window.addEventListener('keydown', (e) => {
  if (e.code === 'KeyD' || e.code === 'ArrowRight') keys.right = true;
  if (e.code === 'KeyA' || e.code === 'ArrowLeft') keys.left = true;
  if (e.code === 'KeyW' || e.code === 'ArrowUp') keys.up = true;
});

window.addEventListener('keyup', (e) => {
  if (e.code === 'KeyD' || e.code === 'ArrowRight') keys.right = false;
  if (e.code === 'KeyA' || e.code === 'ArrowLeft') keys.left = false;
  if (e.code === 'KeyW' || e.code === 'ArrowUp') keys.up = false;
});

// unfreeze the ball
function releaseBall() {
  ballBody.type = p2.Body.DYNAMIC;
  ballBody.mass = 1;
  ballBody.updateMassProperties();
}

setTimeout(releaseBall, 3000);

let lastTime = performance.now();
const zoom = 40; 

function update() {
  requestAnimationFrame(update);
  const time = performance.now();
  const deltaTime = (time - lastTime) / 1000;
  lastTime = time;

  // apply horizontal movement instantly for arcade feel
  const moveSpeed = 8;
  if (keys.right) {
    playerBody.velocity[0] = moveSpeed;
  } else if (keys.left) {
    playerBody.velocity[0] = -moveSpeed;
  } else {
    playerBody.velocity[0] = 0; 
  }

  // basic jump (checks if moving vertically to prevent double jump)
  if (keys.up && Math.abs(playerBody.velocity[1]) < 0.05 && playerBody.position[1] < -0.5) {
      playerBody.velocity[1] = 10;
      keys.up = false; 
  }

  // step physics world
  world.step(1 / 60, deltaTime, 10);

  // clear screen
  ctx.clearRect(0, 0, canvas.width, canvas.height);

  ctx.save();
  ctx.translate(canvas.width / 2, canvas.height / 2);
  ctx.scale(zoom, -zoom);

  // draw ground
  ctx.beginPath();
  ctx.moveTo(-10, groundBody.position[1]);
  ctx.lineTo(10, groundBody.position[1]);
  ctx.lineWidth = 2 / zoom;
  ctx.strokeStyle = 'black';
  ctx.stroke();

  // draw player (blue)
  ctx.beginPath();
  ctx.arc(playerBody.position[0], playerBody.position[1], playerShape.radius, 0, 2 * Math.PI);
  ctx.fillStyle = 'blue';
  ctx.fill();

  // draw ball (red)
  ctx.beginPath();
  ctx.arc(ballBody.position[0], ballBody.position[1], ballShape.radius, 0, 2 * Math.PI);
  ctx.fillStyle = 'red';
  ctx.fill();

  ctx.restore();
}

update();