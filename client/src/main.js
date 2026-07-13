// init canvas
const canvas = document.getElementById('gameCanvas');
const ctx = canvas.getContext('2d');

const logicalWidth = 1600;
const logicalHeight = 900;
canvas.width = logicalWidth;
canvas.height = logicalHeight;

function resize() {
    const scale = Math.min(
        window.innerWidth / logicalWidth, 
        window.innerHeight / logicalHeight
    );
    canvas.style.width = `${logicalWidth * scale}px`;
    canvas.style.height = `${logicalHeight * scale}px`;
}
window.addEventListener('resize', resize);
resize();

let serverState = null;

// advanced input tracking
const keys = { a: false, d: false, w: false, s: false, dashL: false, dashR: false };
let lastPressA = 0;
let lastPressD = 0;

const socket = new WebSocket("ws://localhost:8080/ws");
socket.onopen = () => console.log("connected to server");
socket.onmessage = (event) => {
    const data = JSON.parse(event.data);
    if (data.type === 'state') serverState = data.state;
};

// handle keydown with double-tap detection
window.addEventListener('keydown', (e) => {
    if (e.repeat) return; // ignore hold-repeats for double tap logic
    
    if (e.code === 'KeyA') {
        const now = Date.now();
        if (now - lastPressA < 250) keys.dashL = true; // 250ms threshold
        lastPressA = now;
        keys.a = true;
    }
    if (e.code === 'KeyD') {
        const now = Date.now();
        if (now - lastPressD < 250) keys.dashR = true;
        lastPressD = now;
        keys.d = true;
    }
    if (e.code === 'KeyW' || e.code === 'Space') keys.w = true;
    if (e.code === 'KeyS') keys.s = true;
    sendInput();
});

// handle keyup
window.addEventListener('keyup', (e) => {
    if (e.code === 'KeyA') keys.a = false;
    if (e.code === 'KeyD') keys.d = false;
    if (e.code === 'KeyW' || e.code === 'Space') keys.w = false;
    if (e.code === 'KeyS') keys.s = false;
    sendInput();
});

window.addEventListener('blur', () => {
    keys.a = keys.d = keys.w = keys.s = false;
    sendInput();
});

// send input as a snapshot
function sendInput() {
    if (socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: 'input', keys: keys }));
        // reset dash trigger immediately after sending
        keys.dashL = false;
        keys.dashR = false;
    }
}

// render frame
function draw() {
    ctx.clearRect(0, 0, canvas.width, canvas.height);

    // draw ground
    ctx.fillStyle = '#eedd82';
    ctx.fillRect(0, canvas.height - 100, canvas.width, 100);

    // draw net
    ctx.fillStyle = '#333';
    ctx.fillRect(canvas.width / 2 - 10, canvas.height - 100 - 240, 20, 240);

    if (serverState) {
        // draw players with rotation support
        for (const id in serverState.players) {
            const p = serverState.players[id];
            
            ctx.save();
            // move context to the center of the player
            ctx.translate(p.pos.x + p.width / 2, p.pos.y + p.height / 2);
            ctx.rotate(p.rotation);
            
            // draw body
            ctx.fillStyle = p.color;
            ctx.fillRect(-p.width / 2, -p.height / 2, p.width, p.height);
            
            // draw "eyes" or visor to clearly see rotation
            ctx.fillStyle = 'rgba(255, 255, 255, 0.8)';
            const eyeOffsetX = p.side === 'left' ? 10 : -50;
            ctx.fillRect(eyeOffsetX, -30, 40, 20);
            
            ctx.restore();
        }

        // draw ball
        const b = serverState.ball;
        ctx.beginPath();
        ctx.arc(b.pos.x, b.pos.y, b.radius, 0, Math.PI * 2);
        ctx.fillStyle = '#ff4444';
        ctx.fill();
        ctx.closePath();

        // draw ui
        ctx.fillStyle = 'rgba(255, 255, 255, 0.8)';
        ctx.font = 'bold 96px Arial, sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText(
            `${serverState.score.left} - ${serverState.score.right}`, 
            canvas.width / 2, 
            150
        );

        if (serverState.state === 'scored') {
            ctx.fillStyle = 'rgba(0, 0, 0, 0.4)';
            ctx.fillRect(0, 0, canvas.width, canvas.height);
            ctx.fillStyle = '#fff';
            ctx.font = 'bold 120px Arial, sans-serif';
            ctx.fillText('POINT!', canvas.width / 2, canvas.height / 2);
        }
    }
}

function loop() {
    draw();
    requestAnimationFrame(loop);
}
loop();