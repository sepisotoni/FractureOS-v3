/* A short, nonblocking startup sequence. No bitmap pack or animation worker. */
static double startup_at;
static int startup_frame=-1;
static int mark_round(int x,int y,int w,int h,int r){int dx,dy;if(x<0||y<0||x>=w||y>=h)return 0;dx=x<r?r-x:x>=w-r?x-(w-r-1):0;dy=y<r?r-y:y>=h-r?y-(h-r-1):0;return dx*dx+dy*dy<=r*r;}
static void fracture_mark(int x,int y,int size){int px,py,h=size*5/4;for(py=0;py<h;py++)for(px=0;px<size;px++){int stem=mark_round(px,py,19,h,9),top=mark_round(px,py,size,19,9),middle=mark_round(px,py-h/2,size-12,17,8),split=h/2-px/3;if((stem||top||middle)&&!(py>=split-2&&py<=split+2))pixels[(y+py)*W+x+px]=py<split?rgb(172,126,248):rgb(66,202,251);}}
static void startup_draw(double elapsed){const char *name=elapsed<0.65?"Fracture":"FractureOS";int y=145;rect(0,0,W,H,rgb(0,0,0));fracture_mark(130,29,64);if(elapsed>=0.65&&elapsed<0.95)y+=(int)((0.95-elapsed)*30);center(y,name,3,rgb(237,240,249));rounded(126,200,68,3,1,rgb(32,35,42));rounded(126,200,(int)(68*(elapsed>1.4?1:elapsed/1.4)),3,1,rgb(157,116,245));}
