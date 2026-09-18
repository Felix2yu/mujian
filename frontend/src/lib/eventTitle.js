// 演出显示名：剧种前缀规则。
// 日历页与列表页曾经各自维护一套规则，导致同一场演出在不同页面叫法不同
// （日历显示「昆剧《牡丹亭》」，卡片显示「牡丹亭」）。这里是唯一真相源。

// 不会作为「剧种前缀」加在剧名前的分类（汇总/类型而非具体剧种）。
const NON_PREPEND_CATEGORIES = new Set(['拼盘', '音乐会', '音乐剧']);

// 超过这个字数的剧名不再前缀剧种，否则标题被撑爆反而难读。
const MAX_PREPEND_NAME_LEN = 14;

/**
 * 决定是否把剧种拼进演出名。
 * @param {string} name 演出名
 * @param {string} categoryName 单一剧种
 * @param {string[]} [categoryNames] 全部剧种（多剧种时不前缀）
 */
export function formatEventTitle(name, categoryName, categoryNames) {
  if (!categoryName || !name) return name;
  if (categoryNames && categoryNames.length > 1) return name;
  if (NON_PREPEND_CATEGORIES.has(categoryName)) return name;
  // 剧名已包含剧种关键词，避免重复
  if (name.includes(categoryName)) return name;
  if ([...name].length > MAX_PREPEND_NAME_LEN) return name;
  const alreadyBracketed = /^《.*》$/.test(name);
  return alreadyBracketed ? `${categoryName} ${name}` : `${categoryName}《${name}》`;
}
