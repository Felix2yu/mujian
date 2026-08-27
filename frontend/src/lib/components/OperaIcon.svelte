<script>
  // 中式戏曲图标：填充彩色「京剧脸谱」
  // 放弃线描/头冠方向——小尺寸下线条看不清、盔头易被误认。
  // 脸谱是中国戏曲最无歧义的符号，靠配色（白脸+红妆+黑眉眼）与
  // 标志性的「凤眼」上扬眼翅即可一眼识别，明暗主题下都清晰。
  let { size = 44, ...rest } = $props();
</script>

<svg
  viewBox="0 0 64 64"
  width={size}
  height={size}
  fill="none"
  aria-hidden="true"
  {...rest}
>
  <!-- 脸谱轮廓（白脸底，素脸基底） -->
  <path
    d="M32 12 C46 12 51 22 51 33 C51 49 42 58 32 58 C22 58 13 49 13 33 C13 22 18 12 32 12 Z"
    fill="#f6efe2"
    stroke="#20232a"
    stroke-width="2"
  />
  <!-- 额心红妆（填充红叶，非细竖纹，规避奥特曼额头晶石） -->
  <path d="M32 16 C36 20 35 27 32 32 C29 27 28 20 32 16 Z" fill="#c2362f" />
  <!-- 眉（黑，上挑） -->
  <path d="M19 30 Q25 26 31 30" stroke="#20232a" stroke-width="2.6" stroke-linecap="round" fill="none" />
  <path d="M45 30 Q39 26 33 30" stroke="#20232a" stroke-width="2.6" stroke-linecap="round" fill="none" />
  <!-- 凤眼（黑，外眼角上扬的杏仁形 + 眼翅） -->
  <path d="M20 38 Q25 35 30 39 Q25 42 20 38 Z" fill="#20232a" />
  <path d="M44 38 Q39 35 34 39 Q39 42 44 38 Z" fill="#20232a" />
  <path d="M20 38 Q16 36 13 32" stroke="#20232a" stroke-width="2.2" stroke-linecap="round" fill="none" />
  <path d="M44 38 Q48 36 51 32" stroke="#20232a" stroke-width="2.2" stroke-linecap="round" fill="none" />
  <!-- 颊红（净角脸谱颊妆） -->
  <circle cx="22" cy="47" r="3.4" fill="#c2362f" opacity="0.9" />
  <circle cx="42" cy="47" r="3.4" fill="#c2362f" opacity="0.9" />
  <!-- 口 -->
  <path d="M28 51 Q32 54 36 51" stroke="#c2362f" stroke-width="2.4" stroke-linecap="round" fill="none" />
</svg>
