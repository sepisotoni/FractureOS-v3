/* Tiny dashboard adornments. The only battery value shown is a real driver state. */
static double link_at,battery_at;
static int battery_state=-1;
static int link_live(void){return link_at>0&&mono()-link_at<15;}
static void dashboard_chrome(void){uint16_t bg=daylight?rgb(245,247,250):face_color(face.background),c=link_live()?theme_color(theme.accent):theme_color(theme.muted);int x=12,y=2;
 /* Two linked rounded loops, or separated loops with a break. */
 rounded(x,y+7,17,10,5,c);rounded(x+2,y+9,13,6,3,bg);rounded(x+10,y+1,17,10,5,c);rounded(x+12,y+3,13,6,3,bg);
 if(link_live())stroke(x+9,y+12,x+17,y+6,c);else{rect(x+10,y+7,6,5,bg);stroke(x+1,y+1,x+5,y+4,c);}
 if(mono()-battery_at>30||battery_at==0){battery_at=mono();battery_state=-1;
#ifdef __arm__
 {int fd=open("/dev/battery",O_RDONLY|O_NONBLOCK);BATTERY_STATUS status;if(fd>=0){memset(&status,0,sizeof status);if(ioctl(fd,IOR_BATTERY_STATUS,&status)==0)battery_state=status.u8ChargeStatus;close(fd);}}
#endif
 }
 c=theme_color(theme.muted);rounded(281,5,23,12,3,c);rounded(283,7,19,8,2,bg);rect(305,8,2,6,c);
 if(battery_state==1)rounded(285,9,15,4,1,theme_color(theme.accent));else if(battery_state==2){stroke(294,6,289,11,c);stroke(289,11,295,11,c);stroke(295,11,291,16,c);}else text(287,1,"?",1,c);
}
