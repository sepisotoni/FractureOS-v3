/* Data-only face packs. Exactly one 170 KB atlas is resident. No legacy UI. */
typedef struct { char atlas[64]; unsigned long hour_color,minute_color,background;
 int time_format,show_date,show_ampm,colon,date_x,date_y,date_scale;
 int layout,blink,hour_only,show_seconds,weight;
 int colon_x,colon_y,colon_gap,colon_size; int digit[4][4]; } Face;
static Face face;
static unsigned char *face_pixels;
static int face_ready;
static int identifier(const char *s){size_t i;if(!s[0]||strlen(s)>63||strstr(s,".."))return 0;
 for(i=0;s[i];i++)if(!((s[i]>='a'&&s[i]<='z')||(s[i]>='0'&&s[i]<='9')||s[i]=='-'||s[i]=='_'||s[i]=='.'))return 0;return 1;}
static int face_number(const char *s,int lo,int hi,int *n){char *end;long v;errno=0;v=strtol(s,&end,10);if(errno||end==s||*end||v<lo||v>hi)return 0;*n=(int)v;return 1;}
static void face_layout(Face *f,int layout){static const int stacked[4][4]={{55,0,100,120},{165,0,100,120},{55,120,100,120},{165,120,100,120}};static const int horizontal[4][4]={{4,35,70,170},{77,35,70,170},{173,35,70,170},{246,35,70,170}};
 f->layout=layout;if(layout==1){memcpy(f->digit,stacked,sizeof stacked);f->colon=0;}else if(layout==2){memcpy(f->digit,horizontal,sizeof horizontal);f->colon=1;f->colon_x=157;f->colon_y=95;f->colon_gap=42;f->colon_size=6;}}
static int face_field(Face *f,const char *key,const char *v){char *end;unsigned long c;int index,x,y,w,h;char extra;
 if(!strcmp(key,"layout")){if(!strcmp(v,"custom"))f->layout=0;else if(!strcmp(v,"stacked"))face_layout(f,1);else if(!strcmp(v,"horizontal"))face_layout(f,2);else return 0;return 1;}
 if(!strcmp(key,"atlas")){if(!identifier(v))return 0;strcpy(f->atlas,v);return 1;}
 if(!strcmp(key,"hour_color")||!strcmp(key,"minute_color")||!strcmp(key,"background")){if(strlen(v)!=6)return 0;c=strtoul(v,&end,16);if(*end||c>0xffffffUL)return 0;
 if(!strcmp(key,"hour_color"))f->hour_color=c;else if(!strcmp(key,"minute_color"))f->minute_color=c;else f->background=c;return 1;}
 if(strlen(key)==6&&!strncmp(key,"digit",5)&&key[5]>='0'&&key[5]<='3'){
 if(sscanf(v,"%d,%d,%d,%d%c",&x,&y,&w,&h,&extra)!=4||x<0||y<0||w<1||h<1||x>319||y>239||w>320-x||h>240-y)return 0;
 index=key[5]-'0';f->digit[index][0]=x;f->digit[index][1]=y;f->digit[index][2]=w;f->digit[index][3]=h;return 1;}
 if(!strcmp(key,"time_format")){if(strcmp(v,"12")&&strcmp(v,"24"))return 0;f->time_format=atoi(v);return 1;}
 #define FIELD(name,lo,hi) if(!strcmp(key,#name))return face_number(v,lo,hi,&f->name)
 FIELD(weight,0,2);FIELD(show_seconds,0,1);FIELD(blink,0,1);FIELD(hour_only,0,1);FIELD(show_date,0,1);FIELD(show_ampm,0,1);FIELD(colon,0,1);FIELD(date_x,0,319);FIELD(date_y,0,233);FIELD(date_scale,1,4);
 FIELD(colon_x,0,319);FIELD(colon_y,0,239);FIELD(colon_gap,1,200);FIELD(colon_size,1,20);
 #undef FIELD
 return 0;}
static int face_read(const char *path,Face *f){FILE *fp=fopen(path,"r");char line[160],key[64],value[80],extra;int valid=1,count=0;if(!fp)return 0;
 while(fgets(line,sizeof line,fp)){if(line[0]=='#'||line[0]=='\n')continue;if(++count>40||sscanf(line,"%63[^=]=%79s %c",key,value,&extra)!=2||!face_field(f,key,value)){valid=0;break;}}
 if(ferror(fp))valid=0;fclose(fp);return valid&&count>=19&&f->atlas[0]&&f->time_format;}
static void face_path(char *p,size_t size,int user){const char *slash=strrchr(config_path,'/');
 if(user)snprintf(p,size,"%.*s/faces/%s.face",slash?(int)(slash-config_path):1,slash?config_path:".",cfg.face);
 else snprintf(p,size,"%s/%s.face",assets_path,cfg.face);}
static int face_atlas(const Face *f,unsigned char **out){char path[640],header[17];FILE *fp;unsigned char *data;
 if(!identifier(f->atlas))return 0;snprintf(path,sizeof path,"%s/%s",assets_path,f->atlas);fp=fopen(path,"rb");if(!fp)return 0;
 /* Fixed canonical PGM header deliberately bounds parsing and allocations. */
 if(fread(header,1,16,fp)!=16||memcmp(header,"P5\n1000 170\n255\n",16)){fclose(fp);return 0;}
 data=malloc(170000);if(!data){fclose(fp);return 0;}
 if(fread(data,1,170000,fp)!=170000||fgetc(fp)!=EOF||ferror(fp)){free(data);fclose(fp);return 0;}fclose(fp);*out=data;return 1;}
static int face_load(void){Face next;char path[640];unsigned char *data=NULL;memset(&next,0,sizeof next);next.weight=1;
 face_path(path,sizeof path,1);if(!face_read(path,&next)){memset(&next,0,sizeof next);next.weight=1;face_path(path,sizeof path,0);if(!face_read(path,&next))return 0;}
 if(!face_atlas(&next,&data))return 0;free(face_pixels);face_pixels=data;face=next;face_ready=1;return 1;}
static int face_save(const Face *f){char path[640],tmp[650];FILE *fp;int i,ok;face_path(path,sizeof path,1);snprintf(tmp,sizeof tmp,"%s.new",path);fp=fopen(tmp,"w");if(!fp)return 0;
 fprintf(fp,"atlas=%s\nhour_color=%06lx\nminute_color=%06lx\nbackground=%06lx\ntime_format=%d\nshow_date=%d\nshow_ampm=%d\ncolon=%d\ndate_x=%d\ndate_y=%d\ndate_scale=%d\ncolon_x=%d\ncolon_y=%d\ncolon_gap=%d\ncolon_size=%d\n",f->atlas,f->hour_color,f->minute_color,f->background,f->time_format,f->show_date,f->show_ampm,f->colon,f->date_x,f->date_y,f->date_scale,f->colon_x,f->colon_y,f->colon_gap,f->colon_size);
 fprintf(fp,"layout=%s\nblink=%d\nhour_only=%d\n",f->layout==1?"stacked":f->layout==2?"horizontal":"custom",f->blink,f->hour_only);fprintf(fp,"show_seconds=%d\nweight=%d\n",f->show_seconds,f->weight);
 for(i=0;i<4;i++)fprintf(fp,"digit%d=%d,%d,%d,%d\n",i,f->digit[i][0],f->digit[i][1],f->digit[i][2],f->digit[i][3]);ok=fflush(fp)==0&&fsync(fileno(fp))==0;if(fclose(fp))ok=0;if(!ok||rename(tmp,path)){unlink(tmp);return 0;}return 1;}
static uint16_t face_color(unsigned long c){int scale=cfg.profile==2?45:100;return rgb(((c>>16)&255)*scale/100,((c>>8)&255)*scale/100,(c&255)*scale/100);}
/* Draw at final geometry; small labels are never shrunk with the framebuffer. */
static void face_draw_height(const struct tm *t,int height){int hour=t->tm_hour,only=face.hour_only&&t->tm_min==0,values[4],i,x,y,top=height<180&&face.show_date?20:0,draw_height=height-top,left=(W-W*draw_height/H)/2;int hour_rects[2][4]={{38,0,120,240},{162,0,120,240}};char date[32];uint16_t bg=face_color(theme.face_colors?theme.background:face.background),label=face_color(theme.face_colors?theme.minute_color:face.minute_color);rect(0,0,W,H,bg);
 if(face.time_format==12){hour%=12;if(!hour)hour=12;}values[0]=hour/10;values[1]=hour%10;values[2]=t->tm_min/10;values[3]=t->tm_min%10;
 for(i=0;i<(only?2:4);i++){int *r=only?hour_rects[i]:face.digit[i],rw=r[2]*draw_height/H,rh=r[3]*draw_height/H,rx=left+r[0]*draw_height/H,ry=top+r[1]*draw_height/H;uint16_t color=face_color(i<2?(theme.face_colors?theme.hour_color:face.hour_color):(theme.face_colors?theme.minute_color:face.minute_color));
 for(y=0;y<rh;y++)for(x=0;x<rw;x++){unsigned int a=0,inv;int ox,oy,at=(ry+y)*W+rx+x;uint16_t dst=pixels[at];
 /* A 0..2px expansion in output space, bounded inside each digit's cell. */
 for(oy=-face.weight;oy<=face.weight;oy++)for(ox=-face.weight;ox<=face.weight;ox++){int xx=x+ox,yy=y+oy;unsigned int v;if(xx<0||xx>=rw||yy<0||yy>=rh)continue;v=face_pixels[(yy*170/rh)*1000+values[i]*100+xx*100/rw];if(v>a)a=v;}
 inv=255-a;pixels[at]=(uint16_t)((((((dst>>11)&31)*inv+((color>>11)&31)*a+127)/255)<<11)|(((((dst>>5)&63)*inv+((color>>5)&63)*a+127)/255)<<5)|(((dst&31)*inv+(color&31)*a+127)/255));}}
 if(face.colon&&!only&&(!face.blink||t->tm_sec%2==0)){int size=face.colon_size*draw_height/H;if(size<1)size=1;rect(left+face.colon_x*draw_height/H,top+face.colon_y*draw_height/H,size,size,face_color(theme.face_colors?theme.hour_color:face.hour_color));rect(left+face.colon_x*draw_height/H,top+(face.colon_y+face.colon_gap)*draw_height/H,size,size,label);}
 if(face.show_date){int tx=left+face.date_x*draw_height/H,ty=top?0:face.date_y*height/H;strftime(date,sizeof date,"%a %d %b",t);if(ui_font&&tx+ui_width(date,face.date_scale)>W-6)tx=W-6-ui_width(date,face.date_scale);if(ty+ui_height(face.date_scale)>height)ty=height-ui_height(face.date_scale);text(tx<0?0:tx,ty,date,face.date_scale,label);}
 if(face.show_seconds){snprintf(date,sizeof date,"%02d",t->tm_sec);text(W-28,height-20,date,1,label);}
 if(face.show_ampm&&face.time_format==12)text(W-28,height-(face.show_seconds?38:20),t->tm_hour<12?"AM":"PM",1,label);}
static void face_draw(const struct tm *t){face_draw_height(t,H);}
static void face_json(char *reply,size_t cap){int i;size_t used=(size_t)snprintf(reply,cap,"{\"atlas\":\"%s\",\"hour_color\":\"%06lx\",\"minute_color\":\"%06lx\",\"background\":\"%06lx\",\"time_format\":%d,\"show_date\":%d,\"show_ampm\":%d,\"colon\":%d,\"date_x\":%d,\"date_y\":%d,\"date_scale\":%d,\"colon_x\":%d,\"colon_y\":%d,\"colon_gap\":%d,\"colon_size\":%d",face.atlas,face.hour_color,face.minute_color,face.background,face.time_format,face.show_date,face.show_ampm,face.colon,face.date_x,face.date_y,face.date_scale,face.colon_x,face.colon_y,face.colon_gap,face.colon_size);
 if(used<cap)used+=(size_t)snprintf(reply+used,cap-used,",\"layout\":\"%s\",\"blink\":%d,\"hour_only\":%d",face.layout==1?"stacked":face.layout==2?"horizontal":"custom",face.blink,face.hour_only);
 for(i=0;i<4&&used<cap;i++)used+=(size_t)snprintf(reply+used,cap-used,",\"digit%d\":\"%d,%d,%d,%d\"",i,face.digit[i][0],face.digit[i][1],face.digit[i][2],face.digit[i][3]);if(used<cap)snprintf(reply+used,cap-used,",\"show_seconds\":%d,\"weight\":%d}\n",face.show_seconds,face.weight);}
