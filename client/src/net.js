// websocket transport. owns the connection and hands incoming snapshots to
// whoever subscribed; knows nothing about rendering or game rules.

const RECONNECT_DELAY_MS = 1000;

// derives the socket url from the page url, so the same build works both
// behind the vite dev proxy and when the go server serves the bundle itself
function socketURL() {
  const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
  return `${scheme}://${location.host}/ws`;
}

export class Connection {
  constructor({ onWelcome, onSnapshot, onStatus }) {
    this.onWelcome = onWelcome;
    this.onSnapshot = onSnapshot;
    this.onStatus = onStatus;

    this.socket = null;
    this.reconnectTimer = null;
    this.lastSentKeys = null;
  }

  connect() {
    this.onStatus('connecting');

    const socket = new WebSocket(socketURL());
    this.socket = socket;

    socket.addEventListener('open', () => {
      this.onStatus('connected');
      // the server keeps the last input it received, so a fresh connection
      // must resend the current key state instead of waiting for a change
      this.lastSentKeys = null;
    });

    socket.addEventListener('message', (event) => this.handleMessage(event.data));

    socket.addEventListener('close', () => {
      this.onStatus('disconnected');
      this.scheduleReconnect();
    });

    // close fires after error too, so reconnecting is handled in one place
    socket.addEventListener('error', () => socket.close());
  }

  scheduleReconnect() {
    if (this.reconnectTimer !== null) return;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, RECONNECT_DELAY_MS);
  }

  handleMessage(raw) {
    let envelope;
    try {
      envelope = JSON.parse(raw);
    } catch {
      return;
    }

    switch (envelope.type) {
      case 'welcome':
        this.onWelcome(envelope.data);
        break;
      case 'state':
        this.onSnapshot(envelope.data.world);
        break;
      default:
        break;
    }
  }

  // sendInput only writes when something actually changed: the server holds
  // the last key state until told otherwise, so resending it every frame
  // would be 60 pointless messages per second
  sendInput(keys) {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) return;

    const encoded = JSON.stringify(keys);
    if (encoded === this.lastSentKeys) return;
    this.lastSentKeys = encoded;

    this.socket.send(JSON.stringify({ type: 'input', data: { keys } }));
  }
}
