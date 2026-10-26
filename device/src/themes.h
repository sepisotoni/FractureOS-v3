/* Small data-only UI themes; selected colors only, no background worker. */
typedef struct {unsigned long background,foreground,muted,accent,surface,hour_color,minute_color;int face_colors;} Theme;
static Theme theme={0x091019,0xe3ebf0,0x647c8b,0x45dab9,0x1e3440,0x4be1fa,0x43b9ff,0};
static int theme_id(const char *s){size_t i;if(!s[0]||strlen(s)>31)return 0;for(i=0;s[i];i++)if(!((s[i]>='a'&&s[i]<='z')||(s[i]>='0'&&s[i]<='9')||s[i]=='-'||s[i]=='_'))return 0;return 1;}
static void theme_path(char *out,size_t cap,int user){const char *root=user?config_path:assets_path,*slash=strrchr(root,'/');snprintf(out,cap,"%.*s/themes/%s.theme",slash?(int)(slash-root):1,slash?root:".",cfg.theme_name);}
static int theme_field(Theme *next,const char *key,const char *value){unsigned long color;char *end;
 if(!strcmp(key,"face_colors")){if(strcmp(value,"0")&&strcmp(value,"1"))return 0;next->face_colors=atoi(value);return 1;}
 if(strlen(value)!=6)return 0;color=strtoul(value,&end,16);if(*end||color>0xffffffUL)return 0;
 #define COLOR(name) if(!strcmp(key,#name)){next->name=color;return 1;}
 COLOR(background);COLOR(foreground);COLOR(muted);COLOR(accent);COLOR(surface);COLOR(hour_color);COLOR(minute_color);
 #undef COLOR
 return 0;}
static int theme_read(const char *path,Theme *next){FILE *f=fopen(path,"r");char line[128],key[40],value[40],extra;int count=0,ok=1;if(!f)return 0;
 while(fgets(line,sizeof line,f)){if(line[0]=='#'||line[0]=='\n')continue;if(++count>16||sscanf(line,"%39[^=]=%39s %c",key,value,&extra)!=2||!theme_field(next,key,value)){ok=0;break;}}
 if(ferror(f))ok=0;fclose(f);return ok&&count==8;}
static int theme_load(void){Theme next=theme;char path[640];theme_path(path,sizeof path,1);if(!theme_read(path,&next)){theme_path(path,sizeof path,0);if(!theme_read(path,&next))return 0;}theme=next;return 1;}
static int theme_save(const Theme *next){char path[640],temp[650];FILE *f;int ok;theme_path(path,sizeof path,1);snprintf(temp,sizeof temp,"%s.new",path);f=fopen(temp,"w");if(!f)return 0;
 fprintf(f,"background=%06lx\nforeground=%06lx\nmuted=%06lx\naccent=%06lx\nsurface=%06lx\nhour_color=%06lx\nminute_color=%06lx\nface_colors=%d\n",next->background,next->foreground,next->muted,next->accent,next->surface,next->hour_color,next->minute_color,next->face_colors);
 ok=fflush(f)==0&&fsync(fileno(f))==0;if(fclose(f))ok=0;if(!ok||rename(temp,path)){unlink(temp);return 0;}return 1;}
static uint16_t theme_color(unsigned long color){int scale=cfg.profile==2?45:100;return rgb(((color>>16)&255)*scale/100,((color>>8)&255)*scale/100,(color&255)*scale/100);}
static void theme_json(char *reply,size_t cap){snprintf(reply,cap,"{\"background\":\"%06lx\",\"foreground\":\"%06lx\",\"muted\":\"%06lx\",\"accent\":\"%06lx\",\"surface\":\"%06lx\",\"hour_color\":\"%06lx\",\"minute_color\":\"%06lx\",\"face_colors\":%d}\n",theme.background,theme.foreground,theme.muted,theme.accent,theme.surface,theme.hour_color,theme.minute_color,theme.face_colors);}
