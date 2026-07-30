import {
  VERSION as PI_VERSION,
  type ExtensionAPI,
} from "@earendil-works/pi-coding-agent";

import {
  registerMatty,
  type MattyHost,
} from "../application/register-matty.ts";
import {
  MATTY_PACKAGE_VERSION,
} from "../domain/package-contract.ts";

function createPiHost(pi: ExtensionAPI): MattyHost {
  return {
    registerCommand(name, command) {
      pi.registerCommand(name, {
        description: command.description,
        handler: async (args, context) => {
          await command.handle(args, (message, level) => {
            context.ui.notify(message, level);
          });
        },
      });
    },
    onSessionStart(handler) {
      pi.on("session_start", async (event, context) => {
        await handler(event, (message, level) => {
          context.ui.notify(message, level);
        });
      });
    },
  };
}

export default function mattyExtension(pi: ExtensionAPI): void {
  registerMatty(createPiHost(pi), {
    packageVersion: MATTY_PACKAGE_VERSION,
    piVersion: PI_VERSION,
    platform: process.platform,
    arch: process.arch,
  });
}
