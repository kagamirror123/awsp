// 各シーンの確認用に、指定秒数の静止画を out/ にまとめて書き出す。
// 使い方: npm run stills
import { execFileSync } from "node:child_process";

const FPS = 30;

// ストーリーボードの各シーンを代表する秒数(タイトル/人間/認証/エージェント/設計/締め)。
const SECONDS = [2, 8, 15, 24, 32, 37];

for (const sec of SECONDS) {
  const frame = Math.round(sec * FPS);
  const out = `out/still-${sec}s.png`;
  console.log(`▶ frame=${frame} (t=${sec}s) -> ${out}`);
  execFileSync(
    "npx",
    ["remotion", "still", "src/index.ts", "AwspDemo", out, `--frame=${frame}`],
    { stdio: "inherit" },
  );
}

console.log("done.");
