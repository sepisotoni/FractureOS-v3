/* Small code-drawn media/brand symbols; no image decoder or resident bitmap. */
enum {SYM_SPOTIFY=20,SYM_PREV,SYM_PLAY,SYM_PAUSE,SYM_NEXT,SYM_VOLUME,SYM_MORE,SYM_LYRICS};
static void spotify_mark(int x,int y,int size){int row,col,k,previous_x,previous_y;uint16_t green=rgb(30,215,96),ink=rgb(0,0,0);rounded(x,y,size,size,size/2,green);
 for(row=0;row<3;row++){previous_x=previous_y=0;for(k=0;k<=16;k++){int px=x+size*(20*16+(62-row*5)*k)/1600,py=y+size*((36+row*17)*256-12*k*(16-k)+10*k*16)/25600;if(k)stroke(previous_x,previous_y,px,py,ink);previous_x=px;previous_y=py;}}
 /* Keep tiny variants recognizable without storing a separate sprite. */
 (void)col;}
static void media_symbol(int x,int y,int size,int kind,uint16_t c,uint16_t bg){int i,half=size/2;
 if(kind==SYM_SPOTIFY){spotify_mark(x,y,size);return;}
 if(kind==SYM_PLAY){for(i=0;i<size-4;i++){int span=i<half-2?i:size-5-i;rect(x+4,y+2+i,span+2,1,c);}return;}
 if(kind==SYM_PAUSE){rounded(x+4,y+3,size/5,size-6,1,c);rounded(x+size*3/5,y+3,size/5,size-6,1,c);return;}
 if(kind==SYM_PREV||kind==SYM_NEXT){int flip=kind==SYM_PREV;for(i=0;i<size-6;i++){int span=i<(size-6)/2?i:size-7-i;int px=flip?x+size-7-span:x+4;rect(px,y+3+i,span+2,1,c);}rect(flip?x+2:x+size-4,y+3,2,size-6,c);return;}
 if(kind==SYM_VOLUME){rect(x+2,y+size/3,size/5,size/3,c);for(i=0;i<size/3;i++)rect(x+size/4+i,y+half-i,1,i*2+1,c);stroke(x+size*3/4,y+size/3,x+size-2,y+half,c);stroke(x+size-2,y+half,x+size*3/4,y+size*2/3,c);return;}
 if(kind==SYM_MORE){for(i=0;i<3;i++)rounded(x+3+i*(size-6)/3,y+half-2,4,4,2,c);return;}
 if(kind==SYM_LYRICS){rounded(x+1,y+2,size-2,size-5,4,c);rounded(x+3,y+4,size-6,size-9,2,bg);for(i=0;i<3;i++)rect(x+6,y+6+i*4,size-12,1,c);return;}}
static void text_lines(int x,int y,const char *value,int width,int lines,int scale,uint16_t c){char line[161];size_t at=0,n;int row;for(row=0;row<lines&&value[at];row++){n=0;while(value[at+n]&&value[at+n]!='\n'&&n<sizeof line-1){line[n]=value[at+n];line[n+1]=0;if((ui_font?ui_width(line,scale):(int)(n+1)*6*scale)>width)break;n++;}if(!n)n=1;if(value[at+n]&&value[at+n]!='\n'){size_t cut=n;while(cut&&value[at+cut]!=' ')cut--;if(cut>n/2)n=cut;}memcpy(line,value+at,n);line[n]=0;text_fit(x,y+row*(ui_height(scale)+3),line,width,scale,c);at+=n;while(value[at]==' '||value[at]=='\n')at++;}}
