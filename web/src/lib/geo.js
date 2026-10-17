// 坐标系转换（与 backend/internal/geo 保持同算法）。
//
// 库里存的场馆坐标是 GCJ-02（国内地图 / 苹果地图复制出来的值）。
// OpenStreetMap 等国际瓦片与 RFC 5545 ICS GEO 都是 WGS-84，需要转回；
// 高德 / 腾讯瓦片本身就是 GCJ-02，原样打点即可。

const PI = Math.PI;
const A = 6378245.0;
// 与 eviltransform 参考实现及 backend/internal/geo 保持一致的字面量；
// 原写法 0.00669342162296594323 超出 double 精度，运行时会截断为下值，
// 此处直接写成截断后的等价形式（两者 === 为 true）。
const EE = 0.006693421622965943;

function outOfChina(lat, lng) {
  return !(lng > 73.66 && lng < 135.05 && lat > 3.86 && lat < 53.55);
}

function transformLat(x, y) {
  let ret = -100.0 + 2.0 * x + 3.0 * y + 0.2 * y * y + 0.1 * x * y + 0.2 * Math.sqrt(Math.abs(x));
  ret += ((20.0 * Math.sin(6.0 * x * PI) + 20.0 * Math.sin(2.0 * x * PI)) * 2.0) / 3.0;
  ret += ((20.0 * Math.sin(y * PI) + 40.0 * Math.sin((y / 3.0) * PI)) * 2.0) / 3.0;
  ret += ((160.0 * Math.sin((y / 12.0) * PI) + 320 * Math.sin((y * PI) / 30.0)) * 2.0) / 3.0;
  return ret;
}

function transformLng(x, y) {
  let ret = 300.0 + x + 2.0 * y + 0.1 * x * x + 0.1 * x * y + 0.1 * Math.sqrt(Math.abs(x));
  ret += ((20.0 * Math.sin(6.0 * x * PI) + 20.0 * Math.sin(2.0 * x * PI)) * 2.0) / 3.0;
  ret += ((20.0 * Math.sin(x * PI) + 40.0 * Math.sin((x / 3.0) * PI)) * 2.0) / 3.0;
  ret += ((150.0 * Math.sin((x / 12.0) * PI) + 300.0 * Math.sin((x / 30.0) * PI)) * 2.0) / 3.0;
  return ret;
}

export function wgs84ToGcj02(lat, lng) {
  if (outOfChina(lat, lng)) return [lat, lng];
  let dLat = transformLat(lng - 105.0, lat - 35.0);
  let dLng = transformLng(lng - 105.0, lat - 35.0);
  const radLat = (lat / 180.0) * PI;
  let magic = Math.sin(radLat);
  magic = 1 - EE * magic * magic;
  const sqrtMagic = Math.sqrt(magic);
  dLat = (dLat * 180.0) / (((A * (1 - EE)) / (magic * sqrtMagic)) * PI);
  dLng = (dLng * 180.0) / ((A / sqrtMagic) * Math.cos(radLat) * PI);
  return [lat + dLat, lng + dLng];
}

export function gcj02ToWgs84(lat, lng) {
  if (outOfChina(lat, lng)) return [lat, lng];
  let wLat = lat;
  let wLng = lng;
  for (let i = 0; i < 2; i++) {
    const [gLat, gLng] = wgs84ToGcj02(wLat, wLng);
    wLat -= gLat - lat;
    wLng -= gLng - lng;
  }
  return [wLat, wLng];
}
