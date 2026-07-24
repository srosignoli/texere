// imageStyles maps an image cell's natural size + non-destructive edit metadata
// (rotation in 90° steps, normalized crop of the ROTATED frame) to CSS for three
// nested layers:
//   <div style={viewport}>        // clips to the crop; aspect-ratio = crop box
//     <div style={pan}>           // the full rotated image, panned so the crop fills the viewport
//       <img style={img}>         // the natural image, rotated to fill the rotated frame
//
// All sizes are percentages, so it is resolution-independent, and it never
// distorts (each layer's aspect matches its content). crop is {x,y,w,h} fractions
// 0..1 of the rotated frame, or null for the whole image.
export function imageStyles({ naturalW, naturalH, rotation = 0, crop = null }) {
  const W = naturalW || 1
  const H = naturalH || 1
  const r = ((Math.round(rotation / 90) * 90) % 360 + 360) % 360
  const swap = r === 90 || r === 270
  const RW = swap ? H : W // rotated-frame dimensions
  const RH = swap ? W : H
  const c = crop && crop.w > 0 && crop.h > 0 ? crop : { x: 0, y: 0, w: 1, h: 1 }

  const viewport = `position:relative;overflow:hidden;width:100%;` +
    `aspect-ratio:${c.w * RW} / ${c.h * RH};`
  const pan = `position:absolute;` +
    `left:${(-c.x / c.w) * 100}%;top:${(-c.y / c.h) * 100}%;` +
    `width:${100 / c.w}%;height:${100 / c.h}%;`
  const img = swap
    ? `position:absolute;top:50%;left:50%;` +
      `width:${(W / H) * 100}%;height:${(H / W) * 100}%;` +
      `transform:translate(-50%,-50%) rotate(${r}deg);display:block;`
    : `display:block;width:100%;height:100%;transform:rotate(${r}deg);`

  return { viewport, pan, img }
}
