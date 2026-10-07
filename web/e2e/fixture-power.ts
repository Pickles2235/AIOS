import {chmodSync, mkdirSync, writeFileSync} from "node:fs";
import {join} from "node:path";

// These browser fixtures exercise indexing, not host battery policy. Keep the
// probe override inside their disposable source root and the spawned backend.
export function acPowerEnvironment(root: string): NodeJS.ProcessEnv {
  const bin = join(root, "test-bin");
  mkdirSync(bin, {recursive:true});
  const probe = join(bin, "pmset");
  writeFileSync(probe, "#!/bin/sh\nprintf \"Now drawing from 'AC Power'\\n\"\n");
  chmodSync(probe, 0o700);
  return {...process.env, PATH: `${bin}:${process.env.PATH || ""}`};
}
