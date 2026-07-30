import { acquireRepositoryWriter } from "../../src/application/single-writer.ts";

const workingTree = process.argv[2];
const stateRoot = process.argv[3];
const release = await acquireRepositoryWriter(workingTree, stateRoot);
if (!release) {
  process.exit(2);
}
process.stdout.write("acquired\n");
process.stdin.resume();
process.stdin.once("end", async () => {
  await release();
  process.exit(0);
});
