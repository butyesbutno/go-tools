#include <stdio.h>
 
#define __STDC_CONSTANT_MACROS
 
#ifdef _WIN32
//Windows
extern "C"
{
#include "libavcodec/avcodec.h"
#include "libavformat/avformat.h"
#include "libswscale/swscale.h"
#include "SDL2/SDL.h"
};
#else
//Linux...
#ifdef __cplusplus
extern "C"
{
#endif
#include <libavcodec/avcodec.h>
#include <libavformat/avformat.h>
#include <libswscale/swscale.h>
#include <SDL2/SDL.h>
#ifdef __cplusplus
};
#endif
#endif
 
//Refresh Event
#define SFM_REFRESH_EVENT  (SDL_USEREVENT + 1)
 
int thread_exit=0;
 
 //25fps，则延时改为SDL_Delay(40);（1000/25）
int sfp_refresh_thread(void *opaque){
    while (thread_exit == 0) {
		
        SDL_Event event;
		SDL_PollEvent(&event);
		if(event.type == SDL_QUIT) {
			printf("SDL_QUIT detected");
			SDL_PushEvent(&event);
			break;
		}
        event.type = SFM_REFRESH_EVENT;
        SDL_PushEvent(&event);
        SDL_Delay(40);
    }
    return 0;
}

int openRTSPStream(char* filepath, char* title, SDL_Texture* sdlRenderer)
{
    int screen_w=-1,screen_h=-1;
    AVFormatContext    *pFormatCtx;
    int                i, videoindex;
    AVCodecContext    *pCodecCtx;
    AVCodec            *pCodec;
    AVFrame    *pFrame,*pFrameYUV;
    uint8_t *out_buffer;
    AVPacket *packet;
    int ret, got_picture, nalret;
 
    SDL_Event event;
    SDL_Texture* sdlTexture;

    struct SwsContext *img_convert_ctx;
    av_register_all();
    avformat_network_init();
    pFormatCtx = avformat_alloc_context();
 
	AVDictionary* options = NULL;
	av_dict_set(&options, "buffer_size", "1024000", 0);
	av_dict_set(&options, "max_delay", "500000", 0);
	av_dict_set(&options, "fps", "25", 0);              // 确定帧率，演示
	av_dict_set(&options, "stimeout", "20000000", 0);   //设置超时断开连接时间
	av_dict_set(&options, "rtsp_transport", "tcp", 0);  //以udp方式打开，如果以tcp方式打开将udp替换为tcp  

    if(avformat_open_input(&pFormatCtx,filepath,NULL,&options)!=0) {
        printf("Couldn't open input stream.\n");///////////////////////
        return -1;
    }
    if(avformat_find_stream_info(pFormatCtx,NULL)<0) {
        printf("Couldn't find stream information.\n");
        return -1;
    }
    videoindex=-1;
    for(i=0; i<pFormatCtx->nb_streams; i++) 
        if(pFormatCtx->streams[i]->codec->codec_type == AVMEDIA_TYPE_VIDEO) {
            videoindex=i;
            break;
        }
    if(videoindex==-1) {
        printf("Didn't find a video stream.\n");
        return -1;
    }
    pCodecCtx = pFormatCtx->streams[videoindex]->codec;
    pCodec = avcodec_find_decoder(pCodecCtx->codec_id);
    if(pCodec==NULL){
        printf("Codec not found.\n");
        return -1;
    }
    av_opt_set(pCodecCtx->priv_data, "preset", "superfast", 0); // 
    av_opt_set(pCodecCtx->priv_data, "tune", "zerolatency", 0); // 实时编码
    if(avcodec_open2(pCodecCtx, pCodec,NULL)<0){
        printf("Could not open codec.\n");
        return -1;
    }
    pFrame = av_frame_alloc();
    pFrameYUV = av_frame_alloc();
    out_buffer=(uint8_t *)av_malloc(avpicture_get_size(AV_PIX_FMT_YUV420P, pCodecCtx->width, pCodecCtx->height));
    avpicture_fill((AVPicture *)pFrameYUV, out_buffer, AV_PIX_FMT_YUV420P, pCodecCtx->width, pCodecCtx->height);
 
    //Output Info-----------------------------
    printf("---------------- RTSP Information ---------------\n");
    av_dump_format(pFormatCtx, 0, filepath, 0);
    printf("-------------------------------------------------\n");
     
    img_convert_ctx = sws_getContext(pCodecCtx->width, pCodecCtx->height, pCodecCtx->pix_fmt, 
        pCodecCtx->width, pCodecCtx->height, AV_PIX_FMT_YUV420P, SWS_BICUBIC, NULL, NULL, NULL); 

    //SDL 2.0 Support for multiple windows
    screen_w = pCodecCtx->width;
    screen_h = pCodecCtx->height;
    sdlTexture = SDL_CreateTexture(sdlRenderer, SDL_PIXELFORMAT_IYUV, SDL_TEXTUREACCESS_STREAMING, pCodecCtx->width, pCodecCtx->height);  

    packet=(AVPacket *)av_malloc(sizeof(AVPacket));

    for (;;) {
        //Wait
        SDL_WaitEvent(&event);
        if(event.type==SFM_REFRESH_EVENT){
            //------------------------------
            nalret = av_read_frame(pFormatCtx, packet);
            if(nalret >= 0){
                if(packet->stream_index==videoindex){
                    ret = avcodec_decode_video2(pCodecCtx, pFrame, &got_picture, packet);
                    if(ret < 0){
                        printf("Decode Error. ret(%d) nalret(%d)\n", ret, nalret);
                        //break;
                    }
                    if(got_picture){
                        sws_scale(img_convert_ctx, (const uint8_t* const*)pFrame->data, pFrame->linesize, 0, pCodecCtx->height, pFrameYUV->data, pFrameYUV->linesize);
                        //SDL---------------------------
                        SDL_UpdateTexture( sdlTexture, NULL, pFrameYUV->data[0], pFrameYUV->linesize[0] );  
                        SDL_RenderClear( sdlRenderer );  
                        SDL_RenderCopy( sdlRenderer, sdlTexture, NULL, NULL);  
                        SDL_RenderPresent( sdlRenderer );  
                        //SDL End-----------------------
                    }
                }
                av_free_packet(packet);
            } else {
                //Exit Thread
				printf("recreate nalret(%d)\n", nalret);
				break;
            }
        }else if(event.type == SDL_QUIT ){
			printf("receive SDL_QUIT, exit now...\n");
            thread_exit = 1;
            break;
        }
    }
 
    sws_freeContext(img_convert_ctx);

    //--------------
    av_frame_free(&pFrameYUV);
    av_frame_free(&pFrame);
	av_dict_free(&options);
    avcodec_close(pCodecCtx);
    avformat_close_input(&pFormatCtx);
 
    return 0;
}

int main(int argc, char* argv[])
{
    //------------SDL----------------
    SDL_Window *screen; 
    SDL_Renderer* sdlRenderer;
    SDL_Texture* sdlTexture;
    SDL_Rect sdlRect;
    SDL_Thread *video_tid;

    //char filepath[]="rtsp://admin:admin12345@192.168.1.102";
	char *filepath = "rtsp://admin:MIJWVU@192.168.1.136:554/h264/ch1/main/av_stream";
	char *title = "title";
	if(argc > 1) {
		filepath = argv[1];
	}
	if(argc > 2) {
		title = argv[2];
	}
	printf("Playing video:%s\n", filepath);

	if(SDL_Init(SDL_INIT_VIDEO | SDL_INIT_AUDIO | SDL_INIT_TIMER)) {  
        printf( "Could not initialize SDL - %s\n", SDL_GetError()); 
        return -1;
    } 
    screen = SDL_CreateWindow(title, SDL_WINDOWPOS_UNDEFINED, SDL_WINDOWPOS_UNDEFINED,
        1080, 720,SDL_WINDOW_OPENGL);
 
    if(!screen) {  
        printf("SDL: could not create window - exiting:%s\n",SDL_GetError());  
        return -1;
    }
    sdlRenderer = SDL_CreateRenderer(screen, -1, 0);
 
    // sdlRect.x=0;
    // sdlRect.y=0;
    // sdlRect.w = screen_w;
    // sdlRect.h = screen_h;
 
    video_tid = SDL_CreateThread(sfp_refresh_thread, NULL, NULL);

    for(;;) {
        if( openRTSPStream(filepath, title, sdlRenderer) < 0 ) {
            return -1;
        }
        if(thread_exit) break;
    }

    SDL_DetroyWindow(screen);
    SDL_Quit();
    return 0;
}