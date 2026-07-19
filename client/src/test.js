import * as p2 from 'p2-es';

// init physics world with gravity
const world = new p2.World({
  gravity: [0, -9.81]
});

// setup static ground plane
const groundBody = new p2.Body({
  mass: 0
});
const groundShape = new p2.Plane();
groundBody.addShape(groundShape);
world.addBody(groundBody);

// create the hovering ball
const ballBody = new p2.Body({
  mass: 0, // 0 mass keeps it hovering in the air at round start
  position: [0, 5] 
});
const ballShape = new p2.Circle({ radius: 0.5 });
ballBody.addShape(ballShape);
world.addBody(ballBody);

// restore normal mass so the ball drops
function releaseBall() {
  ballBody.mass = 1;
  ballBody.updateMassProperties();
}

// simple game loop for console testing
let lastTime = performance.now();
function update() {
  requestAnimationFrame(update);
  const time = performance.now();
  const deltaTime = (time - lastTime) / 1000;
  lastTime = time;

  // step physics world
  world.step(1 / 60, deltaTime, 10);
  
  // log y-coordinate to check if it drops
  console.log("ball y:", ballBody.position[1]);
}

update();

// drop the ball after 3 seconds
setTimeout(releaseBall, 3000);