/* Bounded, transient calendar summaries supplied by the paired host. */
#define AGENDA_CAPACITY 8
static int agenda_count;
static char agenda_time[AGENDA_CAPACITY][8];
static char agenda_title[AGENDA_CAPACITY][49];
static char agenda_location[AGENDA_CAPACITY][33];

static void agenda_clear(void){
 agenda_count=0;
 memset(agenda_time,0,sizeof agenda_time);
 memset(agenda_title,0,sizeof agenda_title);
 memset(agenda_location,0,sizeof agenda_location);
}
static int agenda_command(const char *name,char *arg){
 char *part[4],*cursor,*comma;
 char *end;long parsed;int index;
 if(!strcmp(name,"agenda-clear")&&!arg){agenda_clear();return 1;}
 if(strcmp(name,"agenda-item")||!arg)return 0;
 cursor=arg;
 for(index=0;index<4;index++){
  part[index]=cursor;
  comma=strchr(cursor,',');
  if(index<3){if(!comma)return 0;*comma=0;cursor=comma+1;}
  else if(comma)return 0;
 }
 errno=0;parsed=strtol(part[0],&end,10);if(errno||end==part[0]||*end||parsed<0||parsed>=AGENDA_CAPACITY)return 0;index=(int)parsed;
 if(!decode_text(part[1],agenda_time[index],sizeof agenda_time[index])||!agenda_time[index][0])return 0;
 if(!decode_text(part[2],agenda_title[index],sizeof agenda_title[index])||!agenda_title[index][0])return 0;
 if(!decode_text(part[3],agenda_location[index],sizeof agenda_location[index]))return 0;
 if(index+1>agenda_count)agenda_count=index+1;
 dirty=1;
 return 1;
}
