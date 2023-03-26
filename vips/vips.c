#include "vips.h"
#include <string.h>

void g_free_go(void **buf) {
  g_free(*buf);
}

void clear_image(VipsImage **image) {
  if (G_IS_OBJECT(*image)) g_clear_object(image); // g_object_unref(image);
}

void swap_and_clear(VipsImage **in, VipsImage *out) {
  clear_image(in);
  *in = out;
}


int vips_initialize() {
    return VIPS_INIT("levmv_vips");
}


int vips_jpegload_go(void *buf, size_t len, VipsImage **out) {
  return vips_jpegload_buffer(buf, len, out, "access", VIPS_ACCESS_SEQUENTIAL, NULL);
}

VipsImage* image_new_from_buffer(void *buf, size_t len) {
    return vips_image_new_from_buffer(buf, len, "", "access", VIPS_ACCESS_SEQUENTIAL, NULL);
}

int thumbnail_buffer(void *buf, size_t len, VipsImage **out, int width, int height, int crop, int size) {
    return vips_thumbnail_buffer(buf, len, out, width, "height", height, "crop", crop, "size", size, NULL);
}

int resize(VipsImage *in, VipsImage **out, double ratio) {
    if (!vips_image_hasalpha(in)) {
        return vips_resize(in, out, ratio);
    }

    VipsImage *base = vips_image_new();
    VipsImage **t = (VipsImage **) vips_object_local_array(VIPS_OBJECT(base), 3);

    int res =
        vips_premultiply(in, &t[0], NULL) ||
        vips_resize(t[0], &t[1], ratio) ||
        vips_unpremultiply(t[1], &t[2], NULL) ||
        vips_cast(t[2], out, in->BandFmt, NULL);

    clear_image(&base);
    return 0;
}

int crop(VipsImage *in, VipsImage **out, int x, int y, int width, int height) {
    return vips_extract_area(in, out, x, y, width, height, NULL);
}


int thumbnail (VipsImage *in, VipsImage **out, int width, int height, int crop, int size) {
    return vips_thumbnail_image (in, out, width,
        "height", height, "crop", crop, "size", size,
        NULL
        );
}


int jpegsave(VipsImage *in, void **buf, size_t *len, int quality) {
    return vips_jpegsave_buffer(
        in, buf, len,
        "Q", quality,
        "optimize_coding", TRUE,
        NULL
    );
}


int webpsave(VipsImage *in, void **buf, size_t *len, int quality) {
    return vips_webpsave_buffer(
        in, buf, len,
        "Q", quality,
        NULL
    );
}


int embed_image(VipsImage *in, VipsImage **out, int left, int top, int width, int height) {
  VipsImage *tmp = NULL;

  if (!vips_image_hasalpha(in)) {
    if (vips_bandjoin_const1(in, &tmp, 255, NULL))
      return 1;

    in = tmp;
  }

  int code = vips_embed(in, out, left, top, width, height, "extend", VIPS_EXTEND_BLACK,  NULL);

  if (tmp) clear_image(&tmp);

  return code;
}


int embed_image_background(VipsImage *in, VipsImage **out, int left, int top, int width,
                int height, double r, double g, double b, double a) {

  double background[3] = {r, g, b};
  double backgroundRGBA[4] = {r, g, b, a};

  VipsArrayDouble *vipsBackground;

  if (in->Bands <= 3) {
    vipsBackground = vips_array_double_new(background, 3);
  } else {
    vipsBackground = vips_array_double_new(backgroundRGBA, 4);
  }

  int code = vips_embed(in, out, left, top, width, height,
    "extend", VIPS_EXTEND_BACKGROUND, "background", vipsBackground, NULL);

  vips_area_unref(VIPS_AREA(vipsBackground));
  return code;
}


int flatten_image(VipsImage *in, VipsImage **out, double r, double g, double b) {

    if (!vips_image_hasalpha(in))
        return vips_copy(in, out, NULL);

    double background[3] = {r, g, b};
    VipsArrayDouble *vipsBackground = vips_array_double_new(background, 3);

    int code = vips_flatten(in, out, "background", vipsBackground);

    vips_area_unref(VIPS_AREA(vipsBackground));
    return code;
}

int composite_image(VipsImage *base, VipsImage *overlay, VipsImage **out) {
    int code = vips_composite2(base, overlay, out, VIPS_BLEND_MODE_OVER, "compositing_space", base->Type, NULL);

    return code;
}


// TODO: maybe use struct for params?
int label(VipsImage *in, VipsImage **out, const char *text, const char *font, const char *font_file, double r, double g, double b, int x, int y, int width, int height) {
    double black[3] = {0, 0, 0};
    double color[3] = { r, g, b };
    VipsObject *base = (VipsObject *) vips_image_new();
    VipsImage **t = (VipsImage **) vips_object_local_array( VIPS_OBJECT( base ), 12 );

    int result = vips_text(&t[0], text, "font", font, "fontfile", font_file, "width", width, "height", height, NULL) ||
         vips_embed(t[0], &t[1], x, y, in->Xsize, in->Ysize, NULL) || // text mask
         NULL == (t[2] = vips_image_new_from_image(in, color, 3)) || // constant image with text color
         NULL == (t[3] = vips_image_new_from_image(in, black, 3)) || // .. with shadow color
         vips_bandjoin2(t[2], t[1], &t[4], NULL) || // text mask as alpha
         vips_bandjoin2(t[3], t[1], &t[5], NULL) ||
         vips_gaussblur(t[5], &t[6], 4, NULL) ||
         vips_composite2(in, t[6], &t[7], VIPS_BLEND_MODE_OVER, "compositing_space", in->Type, NULL) ||
         vips_composite2(t[7], t[4], out, VIPS_BLEND_MODE_OVER, "compositing_space", in->Type, NULL);

    g_object_unref(base);
    return result;
}

int linear(VipsImage *in, VipsImage **out, double multiple, double add) {
    return vips_linear1(in, out, multiple, add, NULL);
}

int strip(VipsImage *in, VipsImage **out) {
  static double default_resolution = 72.0 / 25.4;

  if (vips_copy(in, out,
    "xres", default_resolution,
    "yres", default_resolution,
    NULL
  )) return 1;

  gchar **fields = vips_image_get_fields(in);

  for (int i = 0; fields[i] != NULL; i++) {
    gchar *name = fields[i];

    if (
      (strcmp(name, VIPS_META_ICC_NAME) == 0) ||
      (strcmp(name, "palette-bit-depth") == 0)
    ) continue;

    vips_image_remove(*out, name);
  }

  g_strfreev(fields);

  return 0;
}


void vips_cleanup() {
    vips_error_clear();
    vips_thread_shutdown();
}