const ESC = 0x1b;
const BEL = 0x07;
const ST_C1 = 0x9c;

type ParserState =
  | "text"
  | "escape"
  | "short-escape"
  | "csi"
  | "control-string"
  | "control-string-escape";

export interface TerminalNeutralizer {
  /** Neutralizes one ordered chunk while retaining only constant-size parser state. */
  write(value: string): string;
  /** Discards any unterminated control sequence and resets the parser. */
  end(): void;
}

/** Creates a bounded neutralizer for terminal controls split across ordered chunks. */
export function createTerminalNeutralizer(): TerminalNeutralizer {
  let state: ParserState = "text";
  let osc = false;

  return {
    write(value) {
      let output = "";
      for (let index = 0; index < value.length;) {
        const code = value.charCodeAt(index);

        if (state === "control-string") {
          if ((osc && code === BEL) || code === ST_C1) {
            state = "text";
            index += 1;
          } else if (code === ESC) {
            state = "control-string-escape";
            index += 1;
          } else {
            index += 1;
          }
          continue;
        }

        if (state === "control-string-escape") {
          if (code === 0x5c) {
            state = "text";
            index += 1;
          } else {
            state = "control-string";
          }
          continue;
        }

        if (state === "csi") {
          index += 1;
          if (code >= 0x40 && code <= 0x7e) state = "text";
          continue;
        }

        if (state === "short-escape") {
          if (code >= 0x20 && code <= 0x2f) {
            index += 1;
          } else if (code >= 0x30 && code <= 0x7e) {
            state = "text";
            index += 1;
          } else {
            state = "text";
          }
          continue;
        }

        if (state === "escape") {
          if (code === 0x5d || code === 0x50 || code === 0x58 || code === 0x5e || code === 0x5f) {
            osc = code === 0x5d;
            state = "control-string";
            index += 1;
          } else if (code === 0x5b) {
            state = "csi";
            index += 1;
          } else if (code >= 0x20 && code <= 0x2f) {
            state = "short-escape";
            index += 1;
          } else if (code >= 0x30 && code <= 0x7e) {
            state = "text";
            index += 1;
          } else {
            state = "text";
          }
          continue;
        }

        if (code === 0x90 || code === 0x98 || code === 0x9d || code === 0x9e || code === 0x9f) {
          osc = code === 0x9d;
          state = "control-string";
          index += 1;
          continue;
        }
        if (code === 0x9b) {
          state = "csi";
          index += 1;
          continue;
        }
        if (code === ESC) {
          state = "escape";
          index += 1;
          continue;
        }
        // Preserve tab, LF, and CR as layout; remove every other C0/C1 control.
        if ((code < 0x20 && code !== 0x09 && code !== 0x0a && code !== 0x0d) || (code >= 0x7f && code <= 0x9f)) {
          index += 1;
          continue;
        }
        output += value[index]!;
        index += 1;
      }
      return output;
    },
    end() {
      state = "text";
      osc = false;
    },
  };
}

/** Removes terminal control sequences while preserving printable text and ordinary layout. */
export function neutralizeTerminalText(value: string): string {
  const neutralizer = createTerminalNeutralizer();
  const output = neutralizer.write(value);
  neutralizer.end();
  return output;
}
