/* Small UI text atlas and original geometric symbols. No vector/font runtime. */
static unsigned char *ui_font;
static double menu_anim_start;
static int menu_anim_from,menu_anim_to;
static void ui_font_load(void){char path[640];const char *slash;FILE *f;unsigned char *data;int user;
 for(user=1;user>=0;user--){const char *base=user?config_path:assets_path;slash=strrchr(base,'/');snprintf(path,sizeof path,"%.*s/%s",slash?(int)(slash-base):1,slash?base:".",user?"ui-font.bin":"ui/default.bin");f=fopen(path,"rb");if(!f)continue;data=malloc(45699);if(!data){fclose(f);return;}if(fread(data,1,45699,f)==45699&&!memcmp(data,"FUI1",4)&&fgetc(f)==EOF){int i,ok=1;for(i=0;i<95;i++)if(data[4+i]<1||data[4+i]>20)ok=0;if(ok){ui_font=data;fclose(f);return;}}free(data);fclose(f);}}
static int ui_height(int scale){return scale*9+3;}
static int ui_width(const char *s,int scale){int w=0,height=ui_height(scale);for(;*s;s++){int c=(unsigned char)*s;if(c<32||c>126)c='?';w+=ui_font[4+c-32]*height/24+1;}return w;}
static void ui_text(int x,int y,const char *s,int scale,uint16_t color){int h=ui_height(scale),w=20*h/24,px,py;for(;*s;s++){int c=(unsigned char)*s;if(c<32||c>126)c='?';for(py=0;py<h;py++)for(px=0;px<w;px++){int dx=x+px,dy=y+py;unsigned int a,inv;uint16_t dst;if(dx<0||dx>=W||dy<0||dy>=H)continue;a=ui_font[99+(py*24/h)*1900+(c-32)*20+px*20/w];if(!a)continue;inv=255-a;dst=pixels[dy*W+dx];pixels[dy*W+dx]=(uint16_t)((((((dst>>11)&31)*inv+((color>>11)&31)*a+127)/255)<<11)|(((((dst>>5)&63)*inv+((color>>5)&63)*a+127)/255)<<5)|(((dst&31)*inv+(color&31)*a+127)/255));}x+=ui_font[4+c-32]*h/24+1;}}
static void rounded(int x,int y,int w,int h,int r,uint16_t color){int row,inset,dy;for(row=0;row<h;row++){dy=row<r?r-1-row:row>=h-r?row-(h-r):0;inset=0;if(dy)while(inset<r&&(r-inset)*(r-inset)+dy*dy>r*r)inset++;rect(x+inset,y+row,w-2*inset,1,color);}}
static void stroke(int x,int y,int x2,int y2,uint16_t color){int dx=abs(x2-x),sx=x<x2?1:-1,dy=-abs(y2-y),sy=y<y2?1:-1,e=dx+dy,t;for(;;){rect(x,y,2,2,color);if(x==x2&&y==y2)break;t=2*e;if(t>=dy){e+=dy;x+=sx;}if(t<=dx){e+=dx;y+=sy;}}}
static void icon(int x,int y,int kind,uint16_t c,uint16_t bg){int i;
 if(kind==1){rounded(x+4,y+4,13,13,3,c);rounded(x+6,y+6,9,9,2,bg);for(i=0;i<3;i++){rect(x+6+i*4,y,2,4,c);rect(x+6+i*4,y+18,2,4,c);rect(x,y+6+i*4,4,2,c);rect(x+18,y+6+i*4,4,2,c);}return;}
 if(kind==10){rounded(x,y+4,22,14,3,c);rounded(x+2,y+6,18,10,2,bg);for(i=0;i<3;i++)rect(x+4+i*5,y+8,3,6,c);return;}
 if(kind==2){rounded(x+2,y+1,18,20,9,c);rounded(x+8,y-2,17,18,8,bg);return;}
 if(kind==6){rounded(x+6,y+6,10,10,5,c);for(i=0;i<4;i++){rect(x+10,y+i*7,2,3,c);rect(x+i*7,y+10,3,2,c);}return;}
 if(kind==9){stroke(x+2,y+11,x+11,y+2,c);stroke(x+11,y+2,x+20,y+11,c);rect(x+5,y+10,2,11,c);rect(x+17,y+10,2,11,c);rect(x+5,y+19,14,2,c);return;}
 rounded(x+2,y+2,19,19,9,c);rounded(x+4,y+4,15,15,7,bg);stroke(x+11,y+6,x+11,y+11,c);stroke(x+11,y+11,x+16,y+13,c);
 if(kind==3||kind==4||kind==5)rect(x+8,y-1,7,2,c);
 if(kind==8){stroke(x+2,y+1,x+5,y-1,c);stroke(x+17,y-1,x+20,y+1,c);}}
