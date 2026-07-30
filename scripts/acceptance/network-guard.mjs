import { appendFileSync, writeFileSync } from "node:fs";
import dgram from "node:dgram";
import dns from "node:dns";
import http from "node:http";
import https from "node:https";
import net from "node:net";
import tls from "node:tls";

const readyPath = process.env.MATTY_NETWORK_GUARD_READY;
const violationPath = process.env.MATTY_NETWORK_GUARD_VIOLATION;

if (!readyPath || !violationPath) {
  throw new Error("Matty acceptance network guard paths are missing");
}

writeFileSync(readyPath, "ready\n", "utf8");

function blockNetwork(operation) {
  appendFileSync(violationPath, `${operation}\n`, "utf8");
  throw new Error(`Matty acceptance blocked network operation: ${operation}`);
}

globalThis.fetch = async () => blockNetwork("fetch");
http.request = () => blockNetwork("http.request");
http.get = () => blockNetwork("http.get");
https.request = () => blockNetwork("https.request");
https.get = () => blockNetwork("https.get");
net.connect = () => blockNetwork("net.connect");
net.createConnection = () => blockNetwork("net.createConnection");
net.Socket.prototype.connect = function blockedSocketConnect() {
  return blockNetwork("net.Socket.connect");
};
tls.connect = () => blockNetwork("tls.connect");
dgram.createSocket = () => blockNetwork("dgram.createSocket");
dns.lookup = () => blockNetwork("dns.lookup");
dns.resolve = () => blockNetwork("dns.resolve");
