import * as p2 from 'p2-es';

// init canvas
const canvas = document.createElement('canvas');
const ctx = canvas.getContext('2d');
canvas.width = 800;
canvas.height = 600;
document.body.appendChild(canvas);

// init physics world with gravity
const world = new p2.World({
  gravity: [0, -9.81] // p2 uses standard math coordinates (y goes up)
});

// setup static ground plane
const groundBody = new p2.Body({
  mass: 0,
  position: [0, -2]
});
const groundShape = new p2.Plane();
groundBody.addShape(groundShape);
world.addBody(groundBody);

// create the hovering ball
const ballBody = new p2.Body({
  mass: 0, 
  position: [0, 5] 
});
const ballShape = new p2.Circle({ radius: 0.5 });
ballBody.addShape(ballShape);
world.addBody(ballBody);

// function to unfreeze the ball
function releaseBall() {
  // change type to dynamic so the engine processes gravity
  ballBody.type = p2.Body.DYNAMIC;
  ballBody.mass = 1;
  ballBody.updateMassProperties();
}

// drop the ball after 3 seconds
setTimeout(releaseBall, 3000);

// rendering loop
let lastTime = performance.now();
// zoom factor to convert physics meters to canvas pixels
const zoom = 40; 

function update() {
  requestAnimationFrame(update);
  const time = performance.now();
  const deltaTime = (time - lastTime) / 1000;
  lastTime = time;

  // step physics world
  world.step(1 / 60, deltaTime, 10);

  // clear screen
  ctx.clearRect(0, 0, canvas.width, canvas.height);

  // save context, center camera and flip y axis 
  // (canvas y goes down, but physics y goes up)
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

  // draw ball
  ctx.beginPath();
  ctx.arc(ballBody.position[0], ballBody.position[1], ballShape.radius, 0, 2 * Math.PI);
  ctx.fillStyle = 'red';
  ctx.fill();

  ctx.restore();
}

update();