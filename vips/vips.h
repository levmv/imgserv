#include <stdlib.h>
#include <glib.h>
#include <vips/vips.h>
#include <vips/vector.h>



void clear_image(VipsImage **in);
void g_free_go(void **buf);

void swap_and_clear(VipsImage **in, VipsImage *out);

int vips_initialize();

VipsImage* image_new_from_buffer(void *buf, size_t len);
int vips_jpegload_go(void *buf, size_t len, VipsImage **out);
int thumbnail_buffer(void *buf, size_t len, VipsImage **out,int width,int height, int crop, int size);

int thumbnail (VipsImage *in, VipsImage **out, int width, int height, int crop, int size);
int resize(VipsImage *in, VipsImage **out, double ratio);
int crop(VipsImage *in, VipsImage **out, int x, int y, int width, int height);
int jpegsave(VipsImage *in, void **buf, size_t *len, int quality);
int webpsave(VipsImage *in, void **buf, size_t *len, int quality);

int embed_image(VipsImage *in, VipsImage **out, int left, int top, int width, int height);
int embed_image_background(VipsImage *in, VipsImage **out, int left, int top, int width,
                int height, double r, double g, double b, double a);
int composite_image(VipsImage *base,  VipsImage *overlay, VipsImage **out);
//bool vips_image_hasalpha(VipsImage *in);

int flatten_image(VipsImage *in, VipsImage **out, double r, double g, double b);

int label(VipsImage *in, VipsImage **out, const char *text, const char *font, const char *font_file, double r, double g, double b, int x, int y, int width, int height);
int linear(VipsImage *in, VipsImage **out, double multiple, double add);
int strip(VipsImage *in, VipsImage **out);
void vips_cleanup();