/* Renders the home screen with the real device code, then dumps exactly what the
 * device would export to the PC preview (model + assets) plus its own pixels.
 * usage: preview_drift_device ASSETS_DIR OUT_PREFIX [WALLPAPER_RGB565] [FACE] */
#define main fracture_program_main
#include "../device/src/fracture.c"
#undef main
#include <assert.h>
static void dump(const char *prefix,const char *suffix,const void *data,size_t size){char path[512];FILE *f;snprintf(path,sizeof path,"%s.%s",prefix,suffix);f=fopen(path,"wb");assert(f);if(size)assert(fwrite(data,1,size,f)==size);fclose(f);}
int main(int argc,char **argv){
 char root[]="/tmp/fracture-drift-XXXXXX",path[512],reply[4096],model[8192];
 assert(argc>=3&&mkdtemp(root));
 snprintf(path,sizeof path,"%s/settings.cfg",root);config_path=path;assets_path=argv[1];
 load();theme_load();
 snprintf(model,sizeof model,"face %s",argc>4?argv[4]:"aqua");command(model,reply,sizeof reply);assert(!strcmp(reply,"OK\n")&&face_ready);
 {char blink[]="face-set blink 0";command(blink,reply,sizeof reply);}
 ui_font_load();
 if(argc>3&&argv[3][0]){char buf[W*H*2];FILE *f=fopen(argv[3],"rb");FILE *o;assert(f&&fread(buf,1,sizeof buf,f)==sizeof buf);fclose(f);
  o=fopen("/mnt/sdcard/fracture-v3/wallpaper.rgb","wb");assert(o&&fwrite(buf,1,sizeof buf,o)==sizeof buf);fclose(o);wallpaper_load();assert(wallpaper);}
 else unlink("/mnt/sdcard/fracture-v3/wallpaper.rgb");
 wall_override=1767268800; /* fixed instant: 2026-01-01 12:00:00 UTC */
 render();
 home_model(model,sizeof model);
 dump(argv[2],"model",model,strlen(model));
 dump(argv[2],"dev565",pixels,sizeof pixels);
 dump(argv[2],"atlas",face_pixels,170000);
 dump(argv[2],"font",ui_font,ui_font?45699:0);
 dump(argv[2],"wallpaper",wallpaper,wallpaper?(size_t)W*H*2:0); /* the dimmed buffer, as preview_asset exports it */
 unlink(config_path);rmdir(root);return 0;}
