/* Feeds the exported model+assets to the PC preview renderer and writes RGBA. */
#include "../device/src/preview.c"
#include <assert.h>
static unsigned char *slurp(const char *prefix,const char *suffix,size_t *size){char path[512];FILE *f;unsigned char *d;long n;snprintf(path,sizeof path,"%s.%s",prefix,suffix);f=fopen(path,"rb");assert(f);fseek(f,0,SEEK_END);n=ftell(f);fseek(f,0,SEEK_SET);d=malloc(n+1);assert(d&&fread(d,1,n,f)==(size_t)n);fclose(f);d[n]=0;*size=n;return d;}
int main(int argc,char **argv){size_t m,a,f,w;unsigned char *model,*atlas,*font,*wall,*out;
 assert(argc==3);model=slurp(argv[1],"model",&m);atlas=slurp(argv[1],"atlas",&a);font=slurp(argv[1],"font",&f);wall=slurp(argv[1],"wallpaper",&w);
 assert(FracturePreviewAssets(atlas,(int)a,f?font:NULL,(int)f));
 assert(FracturePreviewWallpaper(w?wall:NULL,(int)w));
 out=malloc(W*H*4);assert(FracturePreviewRender((const char*)model,0,out,W*H*4));
 {FILE *o=fopen(argv[2],"wb");assert(o&&fwrite(out,1,W*H*4,o)==(size_t)(W*H*4));fclose(o);}
 FracturePreviewClose();return 0;}
