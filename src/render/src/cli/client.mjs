/**
 * CLI Client - Connects to the dh-render daemon via Unix socket and sends commands.
 */
import { connect } from 'node:net';
import { createHash } from 'node:crypto';
import { existsSync } from 'node:fs';

const DEFAULT_TIMEOUT = 60000; // 60s for long operations like JSAPI loading

/**
 * Compute the socket path for a given server URL.
 */
export function socketPath(serverUrl) {
    if (!serverUrl) {
        // Look for active session file
        const activePath = '/tmp/dh-render-active.sock';
        if (existsSync(activePath)) return activePath;
        return null;
    }
    const hash = createHash('md5').update(serverUrl).digest('hex').slice(0, 8);
    return `/tmp/dh-render-${hash}.sock`;
}

/**
 * Send a command to the daemon and return the response.
 *
 * @param {string} sockPath - Unix socket path
 * @param {object} command - The command object { cmd, ...args }
 * @param {number} [timeout] - Timeout in ms
 * @returns {Promise<object>} The response
 */
export function sendCommand(sockPath, command, timeout = DEFAULT_TIMEOUT) {
    return new Promise((resolve, reject) => {
        const socket = connect(sockPath, () => {
            socket.write(JSON.stringify(command) + '\n');
        });

        let data = '';
        const timer = setTimeout(() => {
            socket.destroy();
            reject(new Error(`Timeout waiting for daemon response after ${timeout}ms`));
        }, timeout);

        socket.on('data', (chunk) => {
            data += chunk.toString();
            // Look for complete JSON line
            const newlineIdx = data.indexOf('\n');
            if (newlineIdx >= 0) {
                clearTimeout(timer);
                const line = data.slice(0, newlineIdx);
                socket.end();
                try {
                    resolve(JSON.parse(line));
                } catch (e) {
                    reject(new Error(`Invalid daemon response: ${line}`));
                }
            }
        });

        socket.on('error', (err) => {
            clearTimeout(timer);
            reject(err);
        });

        socket.on('end', () => {
            clearTimeout(timer);
            if (data.trim()) {
                try {
                    resolve(JSON.parse(data.trim()));
                } catch (e) {
                    reject(new Error(`Invalid daemon response: ${data.trim()}`));
                }
            } else {
                reject(new Error('Daemon closed connection without response'));
            }
        });
    });
}

/**
 * Check if the daemon is running by trying to connect to its socket.
 */
export function isDaemonRunning(sockPath) {
    return new Promise((resolve) => {
        if (!sockPath || !existsSync(sockPath)) {
            resolve(false);
            return;
        }
        const socket = connect(sockPath, () => {
            socket.write(JSON.stringify({ cmd: 'ping' }) + '\n');
        });
        let data = '';
        const timer = setTimeout(() => {
            socket.destroy();
            resolve(false);
        }, 2000);
        socket.on('data', (chunk) => {
            data += chunk.toString();
            if (data.includes('\n')) {
                clearTimeout(timer);
                socket.end();
                resolve(true);
            }
        });
        socket.on('error', () => {
            clearTimeout(timer);
            resolve(false);
        });
    });
}
